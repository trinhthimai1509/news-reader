# Personal News Reader

Backend Go + PostgreSQL; frontend HTML/CSS/JavaScript thuần được Go phục vụ cùng origin. Không cần Node, Redis, hàng đợi ngoài hoặc dịch vụ dịch/AI. Docker Compose gồm 2 container: app và PostgreSQL. Chi phí máy chủ chưa được đo.

Trạng thái: đã chạy end-to-end trên máy local với nguồn thật (xem AUDIT_REPORT.md và CHANGE_REPORT.md). Chưa phải bản production. Lượt 9 (2026-10-06): đọc công khai, quản lý nguồn cần đăng nhập quản trị, lọc theo quốc gia của nguồn báo.

## Tài liệu cho người vận hành

- [docs/HUONG-DAN-VAN-HANH-VA-XU-LY-LOI.md](docs/HUONG-DAN-VAN-HANH-VA-XU-LY-LOI.md): khởi động, kiểm tra, tài khoản quản trị, quy trình điều tra lỗi, danh mục lỗi thường gặp, sao lưu/cập nhật/khôi phục, mẫu nhờ hỗ trợ và prompt cho AI.
- [docs/GIAI-THICH-HE-THONG-VA-KY-THUAT.md](docs/GIAI-THICH-HE-THONG-VA-KY-THUAT.md): kiến trúc, luồng dữ liệu, adapter, worker, database, API, bảo mật, lựa chọn kỹ thuật, tài liệu hỗ trợ trình bày.

## Chạy bằng Docker Compose

```sh
cp .env.example .env
# Đặt POSTGRES_PASSWORD, ví dụ: openssl rand -hex 32
# Dùng chuỗi hex cho mật khẩu DB vì nó nằm trong DATABASE_URL.
docker compose up --build -d
```

Mở http://127.0.0.1:8080: đọc tin, tìm kiếm và lọc ngay, không cần mã hay đăng nhập. Lượt lấy tin đầu chạy ngay khi khởi động; toàn văn được lấy dần, tối đa 12 trang bài/site/lượt.

### Tài khoản quản trị (quản lý nguồn)

Chỉ có **một** tài khoản quản trị, không có tài khoản hay mật khẩu mặc định. Khi chưa tạo, app vẫn đọc tin bình thường nhưng không ai vào được màn hình Nguồn tin.

```sh
docker compose exec app /app/server admin create <tên-đăng-nhập>   # tạo lần đầu; hỏi mật khẩu 2 lần, không hiện ký tự
docker compose exec app /app/server admin reset-password           # đổi/quên mật khẩu; huỷ mọi phiên đang mở
docker compose exec app /app/server admin logout-all               # đăng xuất mọi thiết bị
docker compose exec app /app/server admin status                   # xem tên đăng nhập, số phiên còn hạn
```

- Mật khẩu 12–128 ký tự; chỉ lưu hash argon2id (bảng `admin_users`), không lưu plaintext, không in ra log.
- Dùng trong script (không có terminal): mật khẩu đọc từ dòng đầu của stdin, ví dụ `docker compose exec -T app /app/server admin reset-password < file-chua-mat-khau`. Không đặt mật khẩu trên dòng lệnh (lưu vào lịch sử shell).
- Chạy Go trực tiếp: `go run ./cmd/server admin create <tên>` (cần `DATABASE_URL`).
- Tạo lần hai bị từ chối ("đã có tài khoản quản trị"); dùng `reset-password`.

**Đăng nhập:** bấm "Đăng nhập quản trị" ở góc trên, nhập tên và mật khẩu → mở màn hình Nguồn tin. **Đăng xuất:** nút "Đăng xuất"; phiên bị xoá ở server, cookie cũ không dùng lại được.

**Phiên và cookie:** phiên lưu phía server (bảng `admin_sessions`, chỉ lưu SHA-256 của cookie). Cookie `nr_admin`: `HttpOnly`, `SameSite=Strict`, `Path=/`. Hết hạn sau `ADMIN_SESSION_TTL` (mặc định 12h) kể từ lúc đăng nhập, hoặc sau `ADMIN_SESSION_IDLE` (mặc định 2h) không thao tác. Mọi request ghi cần thêm header `X-CSRF-Token` của phiên, cùng origin và body JSON. Sai đăng nhập 5 lần/15 phút từ một địa chỉ (hoặc 30 lần tổng) → 429; thông báo lỗi chung, không cho biết tên đăng nhập có tồn tại hay không.

**Khi triển khai HTTPS** (sau reverse proxy): đặt `COOKIE_SECURE=true` trong `.env` rồi `docker compose up -d app`. Cookie khi đó có `Secure` và tên `__Host-nr_admin` (trình duyệt chỉ gửi qua HTTPS, đúng host). Giữ `APP_BIND=127.0.0.1` và để proxy nối vào cổng loopback. Sau proxy, mọi request có cùng địa chỉ nguồn (proxy), nên giới hạn đăng nhập theo địa chỉ thành giới hạn chung; nên đặt thêm rate limit ở proxy. Proxy phải giữ nguyên header `Host` và `Origin` (kiểm tra CSRF so Origin với Host).

`ADMIN_TOKEN` và `LOCAL_NO_AUTH` của bản trước **không còn tác dụng** (app bỏ qua và ghi log nhắc). Có thể xoá hai dòng này khỏi `.env`.

```sh
docker compose logs -f app   # log
docker compose stop          # dừng, giữ dữ liệu (chạy lại: docker compose start)
docker compose down          # xoá container, giữ volume dữ liệu
```

Không dùng `down -v` (xoá dữ liệu). Cổng app chỉ bind 127.0.0.1 (đổi bằng `APP_PORT`); PostgreSQL không publish ra host. Truy cập từ xa cần reverse proxy HTTPS hoặc SSH tunnel. Image build chạy `go vet` và `go test`; container app chạy user không phải root, rootfs chỉ đọc.

## Chạy Go trực tiếp

Go >= 1.26 và PostgreSQL >= 16:

```sh
export DATABASE_URL='postgres://news:password@localhost:5432/news?sslmode=disable'
export POLL_INTERVAL=2m          # tối thiểu 1m
export FULL_TEXT_ENABLED=true    # false: chỉ lấy RSS
export LISTEN_ADDR=127.0.0.1:8080  # mặc định; container dùng :8080
export COOKIE_SECURE=false        # true khi phục vụ qua HTTPS
export ADMIN_SESSION_TTL=12h ADMIN_SESSION_IDLE=2h   # tuỳ chọn
go test ./...
go run ./cmd/server admin create <tên-đăng-nhập>   # một lần
go run ./cmd/server
```

Chạy từ thư mục gốc vì `web/` và `migrations/` được đọc theo đường dẫn tương đối. Migration trong `migrations/*.sql` được áp dụng một lần mỗi file (bảng `schema_migrations`), có advisory lock nên nhiều instance khởi động cùng lúc vẫn an toàn.

Integration test cần một database **trống, dùng một lần** (mỗi test xoá và tạo lại schema `public`): `TEST_DATABASE_URL=postgres://... go test -p 1 ./cmd/server/`. Test logic frontend: `node --test tests/`.

## Tính năng

- 19 feed: VnExpress (tin mới nhất + 8 chuyên mục), BBC News tiếng Anh (trang chủ + 5 mục) và Tuổi Trẻ Online (thử nghiệm: Thời sự, Thế giới, Kinh doanh, Video). Thêm bằng "Thêm nguồn" với URL như VnExpress `https://vnexpress.net/rss/<chuyen-muc>.rss`, BBC `https://feeds.bbci.co.uk/news/<section>/rss.xml`, Tuổi Trẻ `https://tuoitre.vn/<muc>.rss`, hoặc URL trang chủ. URL khác bị từ chối.
- **Đọc tin công khai:** khách xem danh sách, đọc bài, tìm kiếm và lọc theo chuyên mục/quốc gia/nguồn mà không cần đăng nhập. Quản lý nguồn (import, thêm, sửa tên/chuyên mục/quốc gia, bật/tắt, xoá) chỉ dành cho quản trị viên, được kiểm tra ở backend (`/api/admin/*`).
- **Quốc gia của nguồn báo:** là quốc gia của tòa soạn/website (VnExpress, Tuổi Trẻ → Việt Nam; BBC → Vương quốc Anh), **không phải** quốc gia được nhắc trong bài: bài BBC viết về Việt Nam vẫn thuộc Vương quốc Anh. Quốc gia gán theo website (bảng `publishers`, theo adapter) nên mọi feed của một website luôn cùng quốc gia; quản trị viên chọn/sửa ở khung "Quốc gia của website" trong màn hình Nguồn tin (danh sách sẵn ~66 nước, gồm Thái Lan, Trung Quốc). Website chưa gán hiển thị "Chưa xác định"; không suy đoán từ tên miền, tiêu đề hay AI. Quốc gia khác ngôn ngữ; không dịch bài. Bộ lọc "Quốc gia nguồn" chỉ liệt kê nước đang có nguồn, kết hợp với chuyên mục, nguồn và từ khoá; đổi bộ lọc về trang 1; danh sách nguồn thu theo quốc gia đã chọn.
- Chuyên mục: Thời sự, Thế giới, Kinh doanh, Công nghệ, Giải trí, Thể thao, Sức khỏe, Đời sống, Khác. Mỗi feed được gán một chuyên mục (chọn khi thêm/sửa). Bài thuộc chuyên mục của mọi feed đã liệt kê nó (không đoán từ tiêu đề); feed tổng hợp vào Khác.
- Chấm đỏ trên chip chuyên mục và cạnh tiêu đề bài: có bài hệ thống nhận sau lần cuối bạn mở chuyên mục đó trên trình duyệt này (lưu `localStorage`); không hiện số. Refresh nền, màn hình Tất cả, danh sách đang lọc nguồn/quốc gia/từ khoá và request lỗi không đánh dấu đã xem (mở chuyên mục khi đang lọc quốc gia không xoá chấm đỏ của cả chuyên mục).
- Worker: chạy khi khởi động rồi mỗi 2 phút; advisory lock chống chạy trùng; ETag/Last-Modified. Hai site chạy song song; trong một site các feed đọc lần lượt cách 1s (tối đa 30s/feed), rồi tối đa 12 trang bài cách 1,5s trong 60s; 429/503 thì dừng site đó trong lượt.
- Khử trùng: URL bài được chuẩn hoá (bỏ `utm_*`, `at_*`, fragment; BBC gom về `www.bbc.co.uk`) rồi UNIQUE trong PostgreSQL. Bài VnExpress dạng `<slug>-<mã>.html` khử trùng theo mã bài, nên đổi slug không tạo bài mới (link và tiêu đề đi theo slug mới); bản trùng cũ được hợp nhất khi khởi động.
- Thêm nguồn (màn hình Nguồn tin, chỉ quản trị viên, luồng duy nhất): dán URL trang chủ, RSS hoặc trang chuyên mục đã có ánh xạ của VnExpress/BBC/Tuổi Trẻ rồi bấm "Thêm nguồn"; hệ thống tự nhận diện adapter, tên, chuyên mục (chưa rõ thì Khác), đọc thử RSS, thêm và bật feed, rồi cho worker chạy một lượt. URL bài viết và website khác bị từ chối, không tự crawl. Lỗi có nút "Báo lỗi qua email" (chỉ gửi URL đã nhập; địa chỉ nhận đặt bằng `REPORT_EMAIL`, mặc định như bản trước, chỉ trả cho quản trị viên đã đăng nhập). Tên hiển thị và chuyên mục sửa được trên thẻ nguồn; adapter và URL feed thì không.
- Video: MP4 Tuổi Trẻ (trang /video/) phát trực tiếp từ CDN của nguồn khi bấm "Xem video" (không autoplay, không proxy; CSP media-src chỉ cdn2.tuoitre.vn). Video VnExpress, BBC và video trong bài Tuổi Trẻ chỉ hiện poster + "Xem video tại nguồn". Nhãn "Video" chỉ cho biết bài có video.
- Icon nguồn (VnExpress, BBC, Tuổi Trẻ) lưu local trong `web/icons/`, không tải từ bên thứ ba.
- Toàn văn dạng blocks: đoạn văn và ảnh theo đúng thứ tự (VnExpress `fck_detail`, BBC `data-block="text"|"image"`), kèm tác giả và ảnh đại diện. Caption/credit ảnh tách riêng; bỏ link liên quan, avatar, banner newsletter, poster video, widget. Video, Sounds, crossword, trang lỗi → giữ tóm tắt RSS (kèm ảnh RSS nếu có); thử lại tối đa 3 lần (cách 30 rồi 60 phút).
- Ảnh hiển thị trực tiếp từ CDN của nguồn (`ichef.bbci.co.uk`, `*.vnecdn.net`, `cdn2.tuoitre.vn`; CSP chỉ cho phép các host này), không qua proxy. Ảnh lỗi hiện khung thông báo, nội dung giữ nguyên.
- Bài đã lưu trước khi có ảnh/tác giả được bổ sung dần (tối đa 6 bài/site/lượt, sau tin mới); lỗi không ghi đè nội dung cũ. Tiến độ: màn hình Nguồn tin hoặc `GET /api/admin/status` (cần đăng nhập).
- Đọc bài trong web luôn có tên nguồn và link gốc; nhãn "Toàn văn" / "Tóm tắt · đang chờ" / "Chỉ có tóm tắt".
- Tìm theo tiêu đề (không phân biệt hoa/thường, `%`/`_` là ký tự thường) khi nhấn Enter/Tìm; xoá hết từ khoá (dấu X, phím xoá, khoảng trắng) tự trở về danh sách không lọc từ khoá, giữ chuyên mục và nguồn. Lọc chuyên mục và nguồn kết hợp được, phân trang 30 bài.
- Nguồn: thêm, bật/tắt, xoá mềm. Tắt = dừng lấy tin mới, bài cũ vẫn đọc được. Xoá = ẩn nguồn và bài của nó; thêm lại cùng URL khôi phục nguồn và bài. Bài cũng có trong một feed khác đang bật sẽ chuyển sang feed đó.
- Bảo mật: API đọc tin công khai chỉ nhận GET và chỉ trả dữ liệu cho người đọc (nguồn: id, tên, adapter, quốc gia; không có URL feed, lỗi, ETag, cấu hình hay log). Mọi thao tác ghi và thông tin vận hành nằm dưới `/api/admin/` và cần phiên quản trị + CSRF (xem trên); không còn Bearer token hay chế độ bỏ qua xác thực. Không lưu thông tin xác thực trong localStorage/sessionStorage/URL. Nội dung nguồn hiển thị bằng text (không render HTML); CSP chặt; worker chỉ kết nối tới IP công khai và chỉ theo redirect trong allowlist.

## Giới hạn đã biết

- Một tài khoản quản trị; chưa có đăng ký, nhiều tài khoản, phân vai hay khôi phục mật khẩu qua email (reset bằng CLI trên máy chủ). Giới hạn đăng nhập nằm trong bộ nhớ app (reset khi khởi động lại).
- Quốc gia chỉ gán được cho website đã có adapter (VnExpress, BBC, Tuổi Trẻ). Chọn được Thái Lan, Trung Quốc… nhưng chưa có nguồn nào của các nước đó; thêm nguồn mới cần adapter mới (ngoài phạm vi lượt này).

- Lịch 2 phút là lịch kiểm tra, không phải SLA. Đo ngày 2026-10-05: feed VnExpress "tin mới nhất" chậm ~3 giờ so với trang chủ, feed chuyên mục thì mới. Chưa cam kết SLA 5 phút.
- Feed tổng không chứa mọi bài của mọi chuyên mục; BBC feed tổng có cả Sport/Video/Sounds.
- Không lấy video/bảng/đồ hoạ tương tác; bài đã lấy toàn văn không được cập nhật nếu nguồn sửa bài; `pubDate` của BBC là giờ cập nhật. Tác giả VnExpress chỉ có khi bài có dòng ký tên cuối bài.
- Cấu trúc trang nguồn có thể thay đổi; khi đó bài rơi về trạng thái "Chỉ có tóm tắt".
- RSS công khai không đồng nghĩa với quyền dùng toàn văn. Kiểm tra điều kiện nguồn trước khi dùng công khai hoặc thương mại.

Xem AUDIT_REPORT.md (kết quả audit), VALIDATION.md (bằng chứng kiểm tra), HANDOVER.md (việc tiếp theo).
