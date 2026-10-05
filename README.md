# Personal News Reader

Backend Go + PostgreSQL; frontend HTML/CSS/JavaScript thuần được Go phục vụ cùng origin. Không cần Node, Redis, hàng đợi ngoài hoặc dịch vụ dịch/AI. Docker Compose gồm 2 container: app và PostgreSQL. Chi phí máy chủ chưa được đo.

Trạng thái: đã chạy end-to-end trên máy local với nguồn thật (xem AUDIT_REPORT.md và CHANGE_REPORT.md). Chưa phải bản production.

## Chạy bằng Docker Compose

```sh
cp .env.example .env
# Đặt POSTGRES_PASSWORD và ADMIN_TOKEN (>= 32 ký tự), ví dụ mỗi giá trị: openssl rand -hex 32
# Dùng chuỗi hex cho mật khẩu DB vì nó nằm trong DATABASE_URL.
docker compose up --build -d
```

Mở http://127.0.0.1:8080. Mặc định cần nhập ADMIN_TOKEN (`grep '^ADMIN_TOKEN=' .env | cut -d= -f2- | pbcopy` để chép vào clipboard trên macOS). Lượt lấy tin đầu chạy ngay khi khởi động; toàn văn được lấy dần, tối đa 12 trang bài/site/lượt.

### Dùng local không cần mã

Thêm `LOCAL_NO_AUTH=true` vào `.env` rồi chạy `docker compose up -d app`. App chỉ chấp nhận khi port được publish trên loopback (`APP_BIND=127.0.0.1`, mặc định). Chỉ request từ chính máy này (Host là localhost/127.0.0.1) được bỏ qua mã; request từ LAN, container khác, qua proxy hoặc cross-site vẫn cần mã. Muốn yêu cầu mã trở lại: đặt `LOCAL_NO_AUTH=false` và chạy lại lệnh trên. Không bật sau reverse proxy. Chi tiết: CHANGE_REPORT.md.

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
export ADMIN_TOKEN='replace-with-a-random-token-at-least-32-characters'
export POLL_INTERVAL=2m          # tối thiểu 1m
export FULL_TEXT_ENABLED=true    # false: chỉ lấy RSS
export LISTEN_ADDR=127.0.0.1:8080  # mặc định; container dùng :8080
export LOCAL_NO_AUTH=false        # true chỉ được chấp nhận khi LISTEN_ADDR là loopback
go test ./...
go run ./cmd/server
```

Chạy từ thư mục gốc vì `web/` và `migrations/` được đọc theo đường dẫn tương đối. Migration trong `migrations/*.sql` được áp dụng một lần mỗi file (bảng `schema_migrations`), có advisory lock nên nhiều instance khởi động cùng lúc vẫn an toàn.

Integration test cần một database **trống, dùng một lần** (mỗi test xoá và tạo lại schema `public`): `TEST_DATABASE_URL=postgres://... go test -p 1 ./cmd/server/`. Test logic frontend: `node --test tests/`.

## Tính năng

- 19 feed: VnExpress (tin mới nhất + 8 chuyên mục), BBC News tiếng Anh (trang chủ + 5 mục) và Tuổi Trẻ Online (thử nghiệm: Thời sự, Thế giới, Kinh doanh, Video). Thêm bằng "Thêm nguồn" với URL như VnExpress `https://vnexpress.net/rss/<chuyen-muc>.rss`, BBC `https://feeds.bbci.co.uk/news/<section>/rss.xml`, Tuổi Trẻ `https://tuoitre.vn/<muc>.rss`, hoặc URL trang chủ. URL khác bị từ chối.
- Chuyên mục: Thời sự, Thế giới, Kinh doanh, Công nghệ, Giải trí, Thể thao, Sức khỏe, Đời sống, Khác. Mỗi feed được gán một chuyên mục (chọn khi thêm/sửa). Bài thuộc chuyên mục của mọi feed đã liệt kê nó (không đoán từ tiêu đề); feed tổng hợp vào Khác.
- Chấm đỏ trên chip chuyên mục và cạnh tiêu đề bài: có bài hệ thống nhận sau lần cuối bạn mở chuyên mục đó trên trình duyệt này (lưu `localStorage`); không hiện số. Refresh nền, màn hình Tất cả, danh sách đang lọc nguồn/từ khoá và request lỗi không đánh dấu đã xem.
- Worker: chạy khi khởi động rồi mỗi 2 phút; advisory lock chống chạy trùng; ETag/Last-Modified. Hai site chạy song song; trong một site các feed đọc lần lượt cách 1s (tối đa 30s/feed), rồi tối đa 12 trang bài cách 1,5s trong 60s; 429/503 thì dừng site đó trong lượt.
- Khử trùng: URL bài được chuẩn hoá (bỏ `utm_*`, `at_*`, fragment; BBC gom về `www.bbc.co.uk`) rồi UNIQUE trong PostgreSQL. Bài VnExpress dạng `<slug>-<mã>.html` khử trùng theo mã bài, nên đổi slug không tạo bài mới (link và tiêu đề đi theo slug mới); bản trùng cũ được hợp nhất khi khởi động.
- Thêm nguồn (màn hình Nguồn tin, luồng duy nhất): dán URL trang chủ, RSS hoặc trang chuyên mục đã có ánh xạ của VnExpress/BBC/Tuổi Trẻ rồi bấm "Thêm nguồn"; hệ thống tự nhận diện adapter, tên, chuyên mục (chưa rõ thì Khác), đọc thử RSS, thêm và bật feed, rồi cho worker chạy một lượt. URL bài viết và website khác bị từ chối, không tự crawl. Lỗi có nút "Báo lỗi qua email" (chỉ gửi URL đã nhập). Tên hiển thị và chuyên mục sửa được trên thẻ nguồn; adapter và URL feed thì không.
- Video: MP4 Tuổi Trẻ (trang /video/) phát trực tiếp từ CDN của nguồn khi bấm "Xem video" (không autoplay, không proxy; CSP media-src chỉ cdn2.tuoitre.vn). Video VnExpress, BBC và video trong bài Tuổi Trẻ chỉ hiện poster + "Xem video tại nguồn". Nhãn "Video" chỉ cho biết bài có video.
- Icon nguồn (VnExpress, BBC, Tuổi Trẻ) lưu local trong `web/icons/`, không tải từ bên thứ ba.
- Toàn văn dạng blocks: đoạn văn và ảnh theo đúng thứ tự (VnExpress `fck_detail`, BBC `data-block="text"|"image"`), kèm tác giả và ảnh đại diện. Caption/credit ảnh tách riêng; bỏ link liên quan, avatar, banner newsletter, poster video, widget. Video, Sounds, crossword, trang lỗi → giữ tóm tắt RSS (kèm ảnh RSS nếu có); thử lại tối đa 3 lần (cách 30 rồi 60 phút).
- Ảnh hiển thị trực tiếp từ CDN của nguồn (`ichef.bbci.co.uk`, `*.vnecdn.net`, `cdn2.tuoitre.vn`; CSP chỉ cho phép các host này), không qua proxy. Ảnh lỗi hiện khung thông báo, nội dung giữ nguyên.
- Bài đã lưu trước khi có ảnh/tác giả được bổ sung dần (tối đa 6 bài/site/lượt, sau tin mới); lỗi không ghi đè nội dung cũ. Tiến độ: màn hình Nguồn tin hoặc `GET /api/status`.
- Đọc bài trong web luôn có tên nguồn và link gốc; nhãn "Toàn văn" / "Tóm tắt · đang chờ" / "Chỉ có tóm tắt".
- Tìm theo tiêu đề (không phân biệt hoa/thường, `%`/`_` là ký tự thường) khi nhấn Enter/Tìm; xoá hết từ khoá (dấu X, phím xoá, khoảng trắng) tự trở về danh sách không lọc từ khoá, giữ chuyên mục và nguồn. Lọc chuyên mục và nguồn kết hợp được, phân trang 30 bài.
- Nguồn: thêm, bật/tắt, xoá mềm. Tắt = dừng lấy tin mới, bài cũ vẫn đọc được. Xoá = ẩn nguồn và bài của nó; thêm lại cùng URL khôi phục nguồn và bài. Bài cũng có trong một feed khác đang bật sẽ chuyển sang feed đó.
- Bảo mật: mọi API cần `Authorization: Bearer <token>` (trừ chế độ local ở trên); 10 lần sai/5 phút → 429; token ở sessionStorage của tab, nút Khoá xoá token; nội dung nguồn hiển thị bằng text (không render HTML); CSP chặt; worker chỉ kết nối tới IP công khai và chỉ theo redirect trong allowlist.

## Giới hạn đã biết

- Lịch 2 phút là lịch kiểm tra, không phải SLA. Đo ngày 2026-10-05: feed VnExpress "tin mới nhất" chậm ~3 giờ so với trang chủ, feed chuyên mục thì mới. Chưa cam kết SLA 5 phút.
- Feed tổng không chứa mọi bài của mọi chuyên mục; BBC feed tổng có cả Sport/Video/Sounds.
- Không lấy video/bảng/đồ hoạ tương tác; bài đã lấy toàn văn không được cập nhật nếu nguồn sửa bài; `pubDate` của BBC là giờ cập nhật. Tác giả VnExpress chỉ có khi bài có dòng ký tên cuối bài.
- Cấu trúc trang nguồn có thể thay đổi; khi đó bài rơi về trạng thái "Chỉ có tóm tắt".
- RSS công khai không đồng nghĩa với quyền dùng toàn văn. Kiểm tra điều kiện nguồn trước khi dùng công khai hoặc thương mại.

Xem AUDIT_REPORT.md (kết quả audit), VALIDATION.md (bằng chứng kiểm tra), HANDOVER.md (việc tiếp theo).
