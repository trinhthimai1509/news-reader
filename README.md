# Personal News Reader

Ứng dụng đọc tin cá nhân: tự động lấy tin từ RSS của **VnExpress**, **BBC News** và **Tuổi Trẻ Online**, lấy toàn văn bài (đoạn văn + ảnh theo đúng thứ tự), rồi hiển thị trong một giao diện web gọn, có tìm kiếm và bộ lọc theo chuyên mục, quốc gia của nguồn và nguồn báo.

- **Backend:** Go (1 binary), PostgreSQL.
- **Frontend:** HTML/CSS/JavaScript thuần, được Go phục vụ cùng origin. Không cần Node, bundler, Redis, hàng đợi hay dịch vụ AI/dịch thuật.
- **Triển khai:** Docker Compose gồm 2 container (`app` và `db`).

> **Trạng thái:** đã chạy end-to-end trên máy local với nguồn thật. Chưa phải bản production (xem [Giới hạn đã biết](#giới-hạn-đã-biết)).

## Mục lục

- [Tính năng](#tính-năng)
- [Kiến trúc](#kiến-trúc)
- [Bắt đầu nhanh (Docker Compose)](#bắt-đầu-nhanh-docker-compose)
- [Tài khoản quản trị](#tài-khoản-quản-trị)
- [Cấu hình](#cấu-hình)
- [Chạy Go trực tiếp](#chạy-go-trực-tiếp)
- [Kiểm thử](#kiểm-thử)
- [API](#api)
- [Bảo mật](#bảo-mật)
- [Vận hành](#vận-hành)
- [Giới hạn đã biết](#giới-hạn-đã-biết)
- [Tài liệu khác](#tài-liệu-khác)
- [Giấy phép](#giấy-phép)

## Tính năng

### Cho người đọc (không cần đăng nhập)

- Danh sách tin mới, phân trang 30 bài; đọc bài ngay trong app, luôn kèm tên nguồn và link gốc.
- Nhãn trạng thái nội dung: **Toàn văn**, **Tóm tắt · đang chờ**, **Chỉ có tóm tắt**.
- **Tìm kiếm** theo tiêu đề (không phân biệt hoa/thường) khi nhấn Enter/Tìm; xoá hết từ khoá thì tự trở về danh sách, giữ nguyên bộ lọc khác.
- **Bộ lọc kết hợp được:** chuyên mục, quốc gia của nguồn, nguồn báo, từ khoá.
- **Chuyên mục:** Tất cả, Thời sự, Thế giới, Kinh tế, Khoa học công nghệ, Chính trị, Quân sự, Đời sống, Khác. Bài thuộc chuyên mục của feed chứa nó (không đoán từ tiêu đề).
- **Quốc gia của nguồn:** là quốc gia của toà soạn (VnExpress, Tuổi Trẻ → Việt Nam; BBC → Vương quốc Anh), *không phải* quốc gia được nhắc trong bài. Không suy đoán từ tên miền hay AI.
- **Chấm đỏ bài mới** trên chip chuyên mục và cạnh tiêu đề: có bài mới từ lần cuối bạn mở chuyên mục đó (lưu trong `localStorage` của trình duyệt).
- **Video:** MP4 của Tuổi Trẻ phát trực tiếp khi bấm "Xem video" (không autoplay); video VnExpress/BBC hiện poster + link "Xem video tại nguồn".
- **Thống kê:** "Lượt đọc: N" cho từng bài và "Đang hoạt động: M" (số phiên có hoạt động trong 5 phút). Không lưu IP, user-agent hay tên đăng nhập.
- Ảnh tải trực tiếp từ CDN của nguồn; icon nguồn lưu local trong `web/icons/`.

### Cho quản trị viên

- Màn hình **Nguồn tin**: dán URL trang chủ, RSS hoặc trang chuyên mục của VnExpress/BBC/Tuổi Trẻ → hệ thống tự nhận diện adapter, tên, chuyên mục, đọc thử RSS rồi thêm và bật feed. URL của website khác bị từ chối.
- Sửa tên hiển thị và chuyên mục; bật/tắt (tắt = ngừng lấy tin mới, bài cũ vẫn đọc được); xoá mềm (thêm lại cùng URL sẽ khôi phục nguồn và bài).
- Gán quốc gia cho từng website (danh sách sẵn ~66 nước).
- Xem tiến độ lấy toàn văn / bổ sung ảnh, tác giả.

### Worker lấy tin

- Chạy ngay khi khởi động, sau đó theo `POLL_INTERVAL` (mặc định 2 phút); advisory lock trong PostgreSQL chống chạy trùng; hỗ trợ ETag/Last-Modified.
- Các site chạy song song; trong một site, feed đọc lần lượt cách 1s, sau đó lấy tối đa 12 trang bài cách 1,5s. Gặp 429/503 thì dừng site đó trong lượt.
- **Khử trùng:** URL được chuẩn hoá (bỏ `utm_*`, `at_*`, fragment…) và đặt UNIQUE; bài VnExpress khử trùng theo mã bài nên đổi slug không tạo bài mới.
- **Toàn văn dạng blocks:** đoạn văn và ảnh theo thứ tự, caption/credit tách riêng, bỏ quảng cáo/widget/link liên quan. Trang không trích được (video, crossword, lỗi…) giữ tóm tắt RSS và thử lại tối đa 3 lần.

## Kiến trúc

```
                ┌──────────────── container app ────────────────┐
 Trình duyệt ──▶│ HTTP server (net/http)                         │
  (web/*.js)    │  ├─ /            → file tĩnh trong web/        │
                │  ├─ /api/*       → API đọc tin (công khai, GET)│
                │  └─ /api/admin/* → quản trị (phiên + CSRF)     │
                │ Worker định kỳ ──▶ RSS + trang bài của nguồn   │
                └──────────────────────────┬────────────────────┘
                                           ▼
                                 PostgreSQL (container db)
```

| Thư mục | Nội dung |
| --- | --- |
| `cmd/server/` | Điểm vào: HTTP server, routes, worker, đăng nhập quản trị, CLI `admin`, thống kê, khử trùng |
| `internal/news/` | Đọc RSS, trích toàn văn (VnExpress, BBC, Tuổi Trẻ), nhận diện URL khi thêm nguồn, video |
| `web/` | Giao diện: `index.html`, `style.css`, các module JS, icon nguồn |
| `migrations/` | SQL schema, áp dụng tuần tự một lần mỗi file (bảng `schema_migrations`) |
| `tests/` | Test logic frontend (`node --test`) |
| `scripts/vps-test/` | Script đo tải để chọn cấu hình VPS |
| `docs/` | Tài liệu bổ sung (đánh giá VPS) |

## Bắt đầu nhanh (Docker Compose)

Yêu cầu: Docker và Docker Compose v2.

```sh
cp .env.example .env
# Đặt POSTGRES_PASSWORD trong .env. Nên dùng chuỗi hex vì nó nằm trong DATABASE_URL:
#   openssl rand -hex 32
docker compose up --build -d
```

Mở <http://127.0.0.1:8080>. Lượt lấy tin đầu tiên chạy ngay khi khởi động; toàn văn được lấy dần qua các lượt.

> **Repo nằm trên ổ exFAT/FAT (USB, thẻ nhớ) trên macOS:** macOS tạo file `._*` mỗi khi ghi file, và Docker báo `failed to xattr ... ._<tên file>: operation not permitted`. Xoá chúng trước mỗi lần build:
>
> ```sh
> dot_clean -n .
> docker compose up --build -d
> ```

Image build sẽ chạy `go vet` và `go test`. Container app chạy bằng user không phải root, rootfs chỉ đọc, bỏ mọi capability. PostgreSQL không publish cổng ra host.

## Tài khoản quản trị

Chỉ có **một** tài khoản quản trị và **không có mật khẩu mặc định**. Khi chưa tạo, app vẫn đọc tin bình thường nhưng không ai vào được màn hình Nguồn tin.

```sh
docker compose exec app /app/server admin create <tên-đăng-nhập>   # tạo lần đầu (hỏi mật khẩu 2 lần, không hiện ký tự)
docker compose exec app /app/server admin reset-password           # đổi/quên mật khẩu; huỷ mọi phiên đang mở
docker compose exec app /app/server admin logout-all               # đăng xuất mọi thiết bị
docker compose exec app /app/server admin status                   # xem tên đăng nhập và số phiên còn hạn
```

- Mật khẩu 12–128 ký tự, chỉ lưu hash **argon2id**.
- Dùng trong script: mật khẩu đọc từ dòng đầu của stdin, ví dụ
  `docker compose exec -T app /app/server admin reset-password < file-chua-mat-khau`.
  Không truyền mật khẩu trên dòng lệnh.
- Đăng nhập bằng nút **"Đăng nhập quản trị"** ở góc trên giao diện.

## Cấu hình

Các biến môi trường (đặt trong `.env` khi dùng Docker Compose):

| Biến | Mặc định | Ý nghĩa |
| --- | --- | --- |
| `POSTGRES_PASSWORD` | *(bắt buộc)* | Mật khẩu PostgreSQL (Compose) |
| `DATABASE_URL` | — | Chuỗi kết nối PostgreSQL (khi chạy Go trực tiếp; Compose tự tạo) |
| `POLL_INTERVAL` | `2m` | Chu kỳ lấy tin, tối thiểu `1m` |
| `FULL_TEXT_ENABLED` | `true` | `false` = chỉ lấy RSS, không lấy toàn văn |
| `LISTEN_ADDR` | `127.0.0.1:8080` | Địa chỉ server lắng nghe (container dùng `:8080`) |
| `APP_BIND` / `APP_PORT` | `127.0.0.1` / `8080` | Địa chỉ/cổng publish ra host (Compose) |
| `COOKIE_SECURE` | `false` | Đặt `true` khi phục vụ qua HTTPS |
| `ADMIN_SESSION_TTL` | `12h` | Thời hạn tối đa của phiên quản trị |
| `ADMIN_SESSION_IDLE` | `2h` | Phiên hết hạn sau khoảng không thao tác này |
| `REPORT_EMAIL` | *(có sẵn trong code)* | Địa chỉ nhận cho nút "Báo lỗi qua email" |
| `DB_MEM_LIMIT` / `APP_MEM_LIMIT` | `384m` / `192m` | Giới hạn RAM container (VPS 2 GB: `768m` / `384m`) |

## Chạy Go trực tiếp

Yêu cầu: Go ≥ 1.26, PostgreSQL ≥ 16. Chạy từ thư mục gốc của repo (vì `web/` và `migrations/` được đọc theo đường dẫn tương đối).

```sh
export DATABASE_URL='postgres://news:password@localhost:5432/news?sslmode=disable'
go run ./cmd/server admin create <tên-đăng-nhập>   # một lần
go run ./cmd/server
```

Migration được áp dụng tự động khi khởi động, có advisory lock nên nhiều instance khởi động cùng lúc vẫn an toàn.

## Kiểm thử

```sh
go test ./...                 # unit test Go
node --test tests/            # test logic frontend
```

Integration test cần một database **trống, dùng một lần** (mỗi test xoá và tạo lại schema `public`):

```sh
TEST_DATABASE_URL='postgres://...' go test -p 1 ./cmd/server/
```

## API

API công khai chỉ nhận `GET` (trừ ghi thống kê) và chỉ trả dữ liệu cần cho người đọc.

| Method | Đường dẫn | Mô tả |
| --- | --- | --- |
| GET | `/api/articles` | Danh sách bài (lọc theo chuyên mục, quốc gia, nguồn, từ khoá; phân trang) |
| GET | `/api/articles/{id}` | Chi tiết bài |
| GET | `/api/sources` | Danh sách nguồn (id, tên, adapter, quốc gia) |
| GET | `/api/categories` | Danh sách chuyên mục |
| GET | `/api/countries` | Danh sách quốc gia đang có nguồn |
| GET | `/api/stats` | Lượt đọc, số phiên đang hoạt động |
| POST | `/api/stats/events` | Ghi sự kiện thống kê (cùng origin, có giới hạn tần suất) |
| GET | `/api/admin/session` | Trạng thái đăng nhập hiện tại |
| GET | `/healthz` | Health check |

Các route quản trị (cần phiên + header `X-CSRF-Token`): `POST /api/admin/login`, `POST /api/admin/logout`, `GET /api/admin/sources`, `POST /api/admin/sources/import`, `PATCH|DELETE /api/admin/sources/{id}`, `GET /api/admin/publishers`, `PATCH /api/admin/publishers/{adapter}`, `GET /api/admin/status`.

## Bảo mật

- Phiên quản trị lưu phía server (chỉ lưu SHA-256 của cookie). Cookie `nr_admin`: `HttpOnly`, `SameSite=Strict`; với `COOKIE_SECURE=true` đổi thành `__Host-nr_admin` có `Secure`.
- Mọi request ghi của quản trị cần cùng origin, body JSON và header `X-CSRF-Token`.
- Giới hạn đăng nhập sai: 5 lần/15 phút mỗi địa chỉ (30 lần tổng) → 429; thông báo lỗi không tiết lộ tên đăng nhập có tồn tại hay không.
- Nội dung từ nguồn hiển thị dạng text (không render HTML); CSP chặt, chỉ cho phép ảnh/video từ CDN của các nguồn đã biết.
- Worker chỉ kết nối tới IP công khai và chỉ theo redirect trong allowlist.
- Không lưu thông tin xác thực trong `localStorage`, `sessionStorage` hay URL.

### Triển khai qua HTTPS

Đặt app sau reverse proxy HTTPS, giữ `APP_BIND=127.0.0.1`, đặt `COOKIE_SECURE=true` rồi `docker compose up -d app`. Proxy phải giữ nguyên header `Host` và `Origin`. Sau proxy mọi request có cùng địa chỉ nguồn, nên đặt thêm rate limit ở proxy (đặc biệt cho `POST /api/admin/login`).

## Vận hành

```sh
docker compose logs -f app    # xem log
docker compose stop           # dừng, giữ dữ liệu (chạy lại: docker compose start)
docker compose down           # xoá container, giữ volume dữ liệu
docker compose exec db pg_dump -U news -Fc news > backup.dump   # sao lưu
```

**Không** dùng `docker compose down -v` (lệnh này xoá dữ liệu). Log Docker bị giới hạn 3 × 10 MB mỗi container.

## Giới hạn đã biết

- Chu kỳ 2 phút là lịch kiểm tra, không phải SLA: feed VnExpress "tin mới nhất" từng chậm ~3 giờ so với trang chủ.
- Chỉ hỗ trợ 3 website có adapter (VnExpress, BBC, Tuổi Trẻ); thêm website mới cần viết adapter.
- Một tài khoản quản trị; chưa có nhiều tài khoản, phân vai hay khôi phục mật khẩu qua email.
- Giới hạn tần suất (đăng nhập, thống kê) nằm trong bộ nhớ, reset khi khởi động lại; sau reverse proxy trở thành giới hạn chung.
- Thống kê không chặn được mọi bot; lượt đọc không phải số người.
- Cấu trúc trang nguồn có thể thay đổi; khi đó bài rơi về trạng thái "Chỉ có tóm tắt".
- Không lấy bảng/đồ hoạ tương tác; bài đã lấy toàn văn không được cập nhật nếu nguồn sửa bài.
- **RSS công khai không đồng nghĩa với quyền dùng toàn văn.** Kiểm tra điều khoản của từng nguồn trước khi dùng công khai hoặc thương mại.

## Tài liệu khác

- [HANDOVER.md](HANDOVER.md): việc cần làm tiếp và ghi chú cho người phát triển.
- [docs/DANH-GIA-VPS.md](docs/DANH-GIA-VPS.md): số đo RAM/CPU/đĩa để chọn VPS 1 GB hay 2 GB.
- [AUDIT_REPORT.md](AUDIT_REPORT.md), [VALIDATION.md](VALIDATION.md), [CHANGE_REPORT.md](CHANGE_REPORT.md): kết quả audit, bằng chứng kiểm tra và lịch sử thay đổi.

## Giấy phép

Mã nguồn phát hành theo giấy phép [MIT](LICENSE).

Giấy phép này **chỉ áp dụng cho mã nguồn** của dự án. Tên, logo (`web/icons/`) và nội dung bài viết lấy từ VnExpress, BBC, Tuổi Trẻ thuộc quyền của chủ sở hữu tương ứng và không nằm trong phạm vi giấy phép MIT.
