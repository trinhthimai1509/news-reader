package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/term"
)

const adminUsage = `Quản lý tài khoản quản trị (chỉ một tài khoản):

  server admin create <tên-đăng-nhập>     tạo tài khoản đầu tiên
  server admin reset-password             đặt mật khẩu mới, đăng xuất mọi phiên
  server admin logout-all                 đăng xuất mọi phiên
  server admin status                     tên đăng nhập và số phiên đang mở

Mật khẩu được hỏi trên terminal (không hiện ký tự, nhập 2 lần) hoặc đọc từ
dòng đầu của stdin khi không có terminal. Cần DATABASE_URL.
`

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{3,64}$`)

// adminCLI runs "server admin ...". It never prints a password or hash.
func adminCLI(args []string, stdin *os.File, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, adminUsage)
		return errors.New("thiếu lệnh")
	}
	ctx := context.Background()
	cfg, e := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if e != nil {
		return errors.New("DATABASE_URL không hợp lệ")
	}
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = migrate(ctx, db, "migrations"); e != nil {
		return fmt.Errorf("migration: %w", e)
	}
	return adminCommand(ctx, db, args, func() (string, error) { return readNewPassword(stdin, out) }, out)
}

func adminCommand(ctx context.Context, db *pgxpool.Pool, args []string, password func() (string, error), out io.Writer) error {
	switch args[0] {
	case "create":
		if len(args) != 2 || !usernamePattern.MatchString(args[1]) {
			return errors.New("dùng: server admin create <tên-đăng-nhập> (3–64 ký tự: chữ, số, . _ -)")
		}
		var n int
		if e := db.QueryRow(ctx, `SELECT count(*) FROM admin_users`).Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return errors.New("đã có tài khoản quản trị; dùng `server admin reset-password` để đổi mật khẩu")
		}
		pw, e := password()
		if e != nil {
			return e
		}
		h, e := hashPassword(pw)
		if e != nil {
			return e
		}
		_, e = db.Exec(ctx, `INSERT INTO admin_users(username,password_hash) VALUES($1,$2)`, args[1], h)
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			return errors.New("đã có tài khoản quản trị; dùng `server admin reset-password` để đổi mật khẩu")
		}
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "Đã tạo tài khoản quản trị %q.\n", args[1])
		return nil
	case "reset-password":
		if len(args) != 1 {
			return errors.New("dùng: server admin reset-password")
		}
		var id int64
		var name string
		e := db.QueryRow(ctx, `SELECT id,username FROM admin_users`).Scan(&id, &name)
		if errors.Is(e, pgx.ErrNoRows) {
			return errors.New("chưa có tài khoản quản trị; dùng `server admin create <tên-đăng-nhập>`")
		}
		if e != nil {
			return e
		}
		pw, e := password()
		if e != nil {
			return e
		}
		h, e := hashPassword(pw)
		if e != nil {
			return e
		}
		tx, e := db.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, `UPDATE admin_users SET password_hash=$2,password_changed_at=now() WHERE id=$1`, id, h); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `DELETE FROM admin_sessions WHERE admin_id=$1`, id)
		if e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		fmt.Fprintf(out, "Đã đổi mật khẩu của %q; %d phiên đăng nhập đã bị huỷ.\n", name, tag.RowsAffected())
		return nil
	case "logout-all":
		tag, e := db.Exec(ctx, `DELETE FROM admin_sessions`)
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "Đã huỷ %d phiên đăng nhập.\n", tag.RowsAffected())
		return nil
	case "status":
		var name string
		var sessions int
		e := db.QueryRow(ctx, `SELECT u.username,(SELECT count(*) FROM admin_sessions s WHERE s.admin_id=u.id AND s.expires_at>now()) FROM admin_users u`).Scan(&name, &sessions)
		if errors.Is(e, pgx.ErrNoRows) {
			fmt.Fprintln(out, "Chưa có tài khoản quản trị.")
			return nil
		}
		if e != nil {
			return e
		}
		fmt.Fprintf(out, "Tài khoản quản trị: %q; phiên chưa hết hạn: %d.\n", name, sessions)
		return nil
	}
	fmt.Fprint(out, adminUsage)
	return fmt.Errorf("lệnh không hợp lệ: %q", args[0])
}

// readNewPassword asks twice without echo on a terminal; otherwise it reads
// the first line of stdin (for scripted use).
func readNewPassword(in *os.File, out io.Writer) (string, error) {
	var pw string
	if fd := int(in.Fd()); term.IsTerminal(fd) {
		fmt.Fprint(out, "Mật khẩu mới: ")
		a, e := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if e != nil {
			return "", e
		}
		fmt.Fprint(out, "Nhập lại: ")
		b, e := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if e != nil {
			return "", e
		}
		if string(a) != string(b) {
			return "", errors.New("hai lần nhập không khớp")
		}
		pw = string(a)
	} else {
		line, e := bufio.NewReader(in).ReadString('\n')
		if e != nil && !(errors.Is(e, io.EOF) && line != "") {
			return "", errors.New("không đọc được mật khẩu từ stdin")
		}
		pw = strings.TrimRight(line, "\r\n")
	}
	if e := validPassword(pw); e != nil {
		return "", e
	}
	return pw, nil
}
