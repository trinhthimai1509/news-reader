# Tiếp tục phát triển

Đã xong (2026-10-05): audit code/security, chạy end-to-end local với VnExpress và BBC thật, kiểm tra UI bằng Chrome (AUDIT_REPORT.md). Sau đó: import nguồn bằng URL; xem video (MP4 Tuổi Trẻ, còn lại fallback); nguồn thử Tuổi Trẻ Online (3 feed + Video); chế độ local không cần mã, 9 chuyên mục theo feed, badge bài mới; tác giả, ảnh trong bài, xoá từ khoá tìm kiếm; chấm đỏ bài mới, icon nguồn, khử trùng VnExpress đổi slug (CHANGE_REPORT.md).

Việc còn lại trước khi bàn giao khách:

1. Đo lại độ trễ feed VnExpress `tin-moi-nhat.rss` từ môi trường triển khai (đo local: chậm ~3 giờ). Nếu vẫn chậm, thay bằng các feed chuyên mục khách cần.
2. Đo độ trễ và tỷ lệ toàn văn trong 24 giờ trước khi hứa thời gian cập nhật. Không hứa mọi tin từ một feed tổng.
3. Triển khai sau reverse proxy HTTPS; đặt giới hạn đăng nhập ở proxy; giới hạn tài nguyên container.
4. Sao lưu PostgreSQL theo lịch (`docker compose exec db pg_dump -U news news`) và thử khôi phục.
5. Quy trình đổi ADMIN_TOKEN (sửa `.env`, `docker compose up -d app`).
6. Xem xét quyền sử dụng nội dung toàn văn và điều khoản của nguồn.
7. Tắt `LOCAL_NO_AUTH` (hoặc không đặt) trên mọi môi trường không phải máy cá nhân; xây đăng nhập thật nếu cần nhiều thiết bị/người dùng (trạng thái "đã xem" hiện chỉ lưu theo trình duyệt).
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
16. Import nguồn: danh mục feed đã kiểm tra nằm ở `news.Catalog`; thêm feed vào danh mục (sau khi GET thử) để import trang chủ cấu hình nó. Ánh xạ chuyên mục → RSS chỉ thêm khi nguồn khai báo (ví dụ `<channel><link>`); VnExpress hiện chưa có. Email báo lỗi: `REPORT_TO` trong `web/importer.js`.
