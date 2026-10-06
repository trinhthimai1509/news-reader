# Tiếp tục phát triển

Đã xong (2026-10-05): audit code/security, chạy end-to-end local với VnExpress và BBC thật, kiểm tra UI bằng Chrome (AUDIT_REPORT.md). Sau đó: import nguồn bằng URL; xem video (MP4 Tuổi Trẻ, còn lại fallback); nguồn thử Tuổi Trẻ Online (3 feed + Video); chế độ local không cần mã, 9 chuyên mục theo feed, badge bài mới; tác giả, ảnh trong bài, xoá từ khoá tìm kiếm; chấm đỏ bài mới, icon nguồn, khử trùng VnExpress đổi slug (CHANGE_REPORT.md). Lượt 9 (2026-10-06): lọc theo quốc gia của nguồn báo; đọc tin công khai, quản lý nguồn chỉ cho một quản trị viên đăng nhập bằng phiên server (CHANGE_REPORT.md, Lượt 9).

**Việc cần làm ngay sau lượt 9:** tạo tài khoản quản trị (chưa có tài khoản nào; không có mật khẩu mặc định):
`docker compose exec app /app/server admin create <tên-đăng-nhập>` rồi đăng nhập bằng nút "Đăng nhập quản trị". Quên mật khẩu: `docker compose exec app /app/server admin reset-password` (huỷ mọi phiên). Có thể xoá `ADMIN_TOKEN` và `LOCAL_NO_AUTH` khỏi `.env` (không còn tác dụng).

Việc còn lại trước khi bàn giao khách:

1. Đo lại độ trễ feed VnExpress `tin-moi-nhat.rss` từ môi trường triển khai (đo local: chậm ~3 giờ). Nếu vẫn chậm, thay bằng các feed chuyên mục khách cần.
2. Đo độ trễ và tỷ lệ toàn văn trong 24 giờ trước khi hứa thời gian cập nhật. Không hứa mọi tin từ một feed tổng.
3. Triển khai sau reverse proxy HTTPS: đặt `COOKIE_SECURE=true`, giữ `APP_BIND=127.0.0.1`, proxy giữ nguyên `Host`/`Origin`; đặt thêm giới hạn request cho `POST /api/admin/login` ở proxy (sau proxy mọi request cùng một địa chỉ nguồn); giới hạn tài nguyên container. Chưa mở ra LAN/Internet trong lượt 9.
4. Sao lưu PostgreSQL theo lịch (`docker compose exec db pg_dump -U news news`) và thử khôi phục.
5. Quản trị: một tài khoản, đổi mật khẩu bằng CLI `admin reset-password`; nghi lộ phiên thì `admin logout-all`. Thời hạn phiên: `ADMIN_SESSION_TTL` (12h), `ADMIN_SESSION_IDLE` (2h).
6. Xem xét quyền sử dụng nội dung toàn văn và điều khoản của nguồn.
7. `LOCAL_NO_AUTH`/`ADMIN_TOKEN` đã bị gỡ (lượt 9). Nếu cần nhiều quản trị viên, phân vai hoặc khôi phục mật khẩu qua email thì phải thiết kế thêm (bảng `admin_users` hiện bị giới hạn một dòng bằng unique index `admin_users_single`). Trạng thái "đã xem" vẫn chỉ lưu theo trình duyệt.
8. Bổ sung feed theo danh sách chuyên mục khách cần (BBC Sport cần mở rộng allowlist `/sport/`); theo dõi tải lên nguồn khi thêm feed.
9. Tuỳ chọn: lấy lại toàn văn khi nguồn sửa bài; pin digest image Docker; kiểm tra trên điện thoại thật.
10. Đã xử lý (Lượt 4): bài VnExpress đổi slug dùng mã bài làm `ident`; bản trùng cũ được hợp nhất khi khởi động (`mergeDuplicates`, log "merge duplicates: …"). Nếu log báo nhóm "kept apart", xem tay rồi quyết định. Bản sao lưu trước khi hợp nhất: `backups/` (không đưa vào image).
11. Theo dõi backfill tác giả/ảnh (`GET /api/status`) tới khi `pending` về 0; xem `enrich_error` nếu `failed` tăng. Nếu nguồn đổi CDN ảnh, cập nhật allowlist (`imageHostAllowed`) và CSP cùng lúc.
12. Khi đổi extractor: tăng `news.ExtractVersion` để bài cũ được đọc lại một lần.
13. Icon nguồn nằm ở `web/icons/<adapter>.png` (map trong `siteIcons`, `web/app.js`). Thêm adapter mới thì thêm icon, nếu không UI tự hiện chữ viết tắt.
14. Tuổi Trẻ (thử nghiệm, 3 feed): theo dõi tỷ lệ "Chỉ có tóm tắt" và `content_error` của adapter `tuoitre`. Dạng chưa gặp (emagazine, album `LayoutAlbum`, infographic, tường thuật) cần khảo sát HTML thật rồi mới thêm vào `internal/news/tuoitre.go`. Thêm feed Tuổi Trẻ khác: lấy URL từ https://tuoitre.vn/rss.htm, GET thử, rồi thêm trong màn hình Nguồn tin. Nếu CDN ảnh đổi, cập nhật `ttImageHost`, `imageHost` (app.js) và CSP cùng lúc.

Khi cấu trúc trang nguồn thay đổi: tăng số bài "Chỉ có tóm tắt" là dấu hiệu. Sửa `Extract` trong `internal/news/feed.go` dựa trên HTML thật và thêm test tương ứng.

Không thêm dịch, AI, thanh toán, multi-tenant hoặc crawler website bất kỳ vào phạm vi hiện tại.
15. Video: chỉ MP4 Tuổi Trẻ phát trong app; VnExpress (chặn hotlink) và BBC (embed không hiển thị khi nhúng) là poster + link nguồn. Trước khi bật lại phát cho một nguồn: kiểm tra lại từ trình duyệt thật (không giả Referer), cập nhật `VideoSrc` (Go), `allowed` (`web/video.js`) và `media-src` trong CSP cùng lúc. Quyền sử dụng video chưa được xem xét.
16. Import nguồn: danh mục feed đã kiểm tra nằm ở `news.Catalog`; thêm feed vào danh mục (sau khi GET thử) để import trang chủ cấu hình nó. Ánh xạ chuyên mục → RSS chỉ thêm khi nguồn khai báo (ví dụ `<channel><link>`); VnExpress hiện chưa có. Email báo lỗi: biến môi trường `REPORT_EMAIL` (mặc định trong `defaultReportEmail`, `cmd/server/main.go`); chỉ trả cho quản trị viên qua `/api/admin/session`.
17. Quốc gia của nguồn báo: bảng `publishers` (khoá = adapter, cột `country` → `countries.code`, NULL = "Chưa xác định"). Thêm adapter mới thì thêm một dòng `publishers` trong migration (FK `sources_publisher_fk` bắt buộc). Thêm nước vào danh sách chọn: thêm dòng `countries` (mã ISO 3166-1 alpha-2, tên tiếng Việt, `position`) trong migration mới. Không suy đoán quốc gia từ tên miền/tiêu đề/AI.
18. Phân quyền API: route công khai đăng ký bằng `public(...)` (chỉ GET) trong `routes()`; mọi route ghi/vận hành phải đăng ký bằng `admin(...)` dưới `/api/admin/`. Test `TestGuestCannotReachAdminAPI` liệt kê route quản trị trong `adminRoutes`: thêm route mới thì thêm vào đó.
