# Trạng thái xác minh

Ngày: 2026-10-05. Môi trường: macOS, Docker 29.8 qua Colima, Go 1.27.1 (container), PostgreSQL 17 (container). Chi tiết findings: AUDIT_REPORT.md.

## Build và kiểm tra tĩnh

- `gofmt -l .`: sạch. `go vet ./...`: qua. `go test ./...`: 19 unit test qua.
- 2 integration test (migration đồng thời 4 tiến trình; chuyển chủ bài khi nguồn bị xoá; tìm kiếm literal) qua trên PostgreSQL 17 tạm thời.
- `govulncheck` v1.8.0: không có lỗ hổng (trước khi nâng dependency: 10 lỗ hổng có đường gọi).
- `node --check web/app.js`: qua.
- `docker compose up --build`: build (gồm vet + test) và chạy; app và db healthy.

## Nguồn thật (GET, User-Agent `PersonalNewsReader/0.1`)

- VnExpress RSS `tin-moi-nhat.rss`: HTTP 200 `application/xml`, 38–39 mục. HEAD trả 406 (khác GET) — đây là nguyên nhân báo cáo trước ghi "chưa xác minh".
- VnExpress `thoi-su.rss`: 200, có bài mới trong vài phút.
- Độ trễ: lúc 14:22 (+07) mục mới nhất của `tin-moi-nhat.rss` là 11:32; trang chủ có bài mới hơn. Chỉ đo từ một mạng, cần đo lại ở môi trường triển khai.
- BBC RSS `news/rss.xml`: HTTP 200 `text/xml`, 29 mục, link có tham số tracking `at_*` (đã chuẩn hoá).
- Trang bài kiểm tra tay: VnExpress 5 bài thường, 1 crossword, 1 bài đã gỡ; BBC 3 bài news, 1 sport, 1 video, 1 Sounds, 1 trang giới thiệu app. Đối chiếu tự động 3 bài mỗi nguồn với trang gốc: tiêu đề, giờ, đoạn đầu/cuối.
- Pipeline live: ~180 bài từ 4 nguồn; 149 toàn văn, 19 chờ, 10 chỉ tóm tắt với lý do đúng (video, bài gỡ, crossword, bài đồ hoạ).
- VnExpress trả HTTP 429 khi lấy trang bài dồn dập; sau khi thêm giãn cách và xử lý 429 không còn lặp lại trong lượt kiểm tra tiếp theo.

## End-to-end

Đã kiểm tra: health check theo DB; 401 khi không token/token sai/thiếu `Bearer`; đăng nhập; đọc tin thật; tìm kiếm, lọc, phân trang; đọc bài với nguồn, link gốc và trạng thái nội dung; thêm/bật/tắt/xoá/thêm lại nguồn; từ chối URL ngoài allowlist; restart giữ dữ liệu; nguồn lỗi (404, redirect ra ngoài allowlist) hiển thị lỗi mà app vẫn chạy.

Trình duyệt: Chrome thật, 1280px và 500px (cửa sổ Chrome không nhỏ hơn 500px); tràn ngang đo ở bề rộng nội dung 375px: không có. Chưa thử trên điện thoại thật.

## Chưa xác minh

- SLA cập nhật 5 phút; backlog 24 giờ; mức RAM/CPU.
- Toàn bộ chuyên mục của hai nguồn.
- Triển khai qua HTTPS/reverse proxy; sao lưu/khôi phục.

## Tài liệu nguồn

- https://vnexpress.net/rss
- https://support.bbc.co.uk/platform/feeds/NewsFeeds.htm
- https://feeds.bbci.co.uk/news/rss.xml

## Lượt 2: truy cập local, chuyên mục, bài mới (2026-10-05)

- Go: gofmt sạch, vet qua, 25 unit test + 7 integration test (PostgreSQL 17 tạm) qua; Node: 7 test qua; govulncheck sạch; Docker build qua.
- Migration 003 trên dữ liệu thật: 180 bài/170 toàn văn cũ giữ nguyên, nguồn không nhân bản, backfill chỉ từ feed đã lưu. Đã `pg_dump` trước khi chạy.
- 13 feed chuyên mục kiểm tra bằng GET (200 XML); lượt đầu với 15 feed: không lỗi, không 429.
- Chế độ local: port chỉ bind 127.0.0.1 (đã kiểm tra docker inspect, lsof, request qua IP LAN bị từ chối); request local không mã 200; Host lạ, X-Forwarded-For, POST cross-site/text-plain, container khác → 401.
- Badge: kiểm chứng với bài thật nhận sau mốc trong Chrome (xuất hiện, giữ sau reload và refresh nền, mất khi mở chuyên mục; request lỗi không đánh dấu).
- Mobile 375px: device emulation qua Chrome DevTools Protocol (headless), không tràn ngang ở 4 màn hình. Chưa thử trên điện thoại thật.
- Chi tiết: CHANGE_REPORT.md.

## Lượt 3: tác giả, ảnh, tìm kiếm (2026-10-05)

- Go: gofmt/vet sạch, 23 unit test `internal/news` + 21 test server (gồm integration trên PostgreSQL 17 tạm) qua; Node: 12 test qua; govulncheck sạch; Docker build qua.
- Migration 004 trên dữ liệu thật: 696 bài trước/sau, bài cũ đọc được qua blocks dạng đoạn văn. Đã `pg_dump` trước.
- Đối chiếu DB với trang gốc (4 VnExpress + 4 BBC): tác giả 8/8, URL ảnh 14/14, caption 11/11, thứ tự 130/133 khối (3 không định vị được do thẻ inline).
- Chrome thật: ảnh tải từ CDN, 0 vi phạm CSP; ảnh lỗi không vỡ bố cục; bấm X native/Cmd+A+Delete/Backspace/khoảng trắng → 1 request q rỗng, giữ chuyên mục/nguồn, không đánh dấu đã xem.
- Mobile 375px (device emulation CDP): không tràn ngang, ảnh co theo màn hình và giữ tỷ lệ.
- Lỗi phát hiện khi chạy thật: panic worker trên slideshow VnExpress (crash-loop ~5 phút, không mất dữ liệu) → đã sửa + recover + test; theo dõi sau sửa: 0 panic, 0 restart.
- Backfill lúc báo cáo: 54 xong, 522 chờ, 0 lỗi.

## Lượt 4: chấm đỏ, icon nguồn, khử trùng VnExpress (2026-10-05)

- Go: gofmt/vet sạch, govulncheck sạch; 25 test `internal/news` + 24 test `cmd/server` (gồm integration trên PostgreSQL 17 tạm) qua; Node 12 qua; Docker build qua.
- Xác minh URL thật: slug cũ (và slug bất kỳ) của bài 5128310, 5128557 → 301 về slug hiện tại.
- Hợp nhất: đã sao lưu bằng `pg_dump` trước; chạy thử trên bản khôi phục rồi live: 2 nhóm tìm thấy, 2 hợp nhất, 0 giữ nguyên, 0 quan hệ mồ côi; chạy lại không đổi.
- Icon: lấy từ `<link rel="icon">` của vnexpress.net và bbc.co.uk/news, lưu local; không request bên thứ ba khi đọc.
- Chrome desktop: chấm 7px, SR label, không còn số/"Mới"/viền đỏ/dòng giải thích; refresh nền giữ chấm; đọc bài, nguồn, tìm kiếm như cũ.
- Mobile 375px (CDP headless, profile tạm): không tràn ngang ở tin/đọc bài/nguồn.
- Worker sau deploy: nhận bài, backfill tiếp tục, 0 panic, 0 restart.
- Chi tiết: CHANGE_REPORT.md (Lượt 4).

## Lượt 5: nguồn Tuổi Trẻ Online (2026-10-05)

- GET từ máy này: `rss.htm` 200 (19 feed); `thoi-su.rss`, `the-gioi.rss`, `kinh-doanh.rss` mỗi feed 200 `text/xml`, 50 item. Slug sai cùng mã bài → 301 về slug đúng.
- Khảo sát 9 trang bài (3 chuyên mục + bài loạt + clip + trang video). Đối chiếu tự động 7 bài lưu trong DB với trang gốc: đoạn văn, ảnh, caption, credit, tác giả, tiêu đề, giờ đăng đều khớp.
- Lúc 09:50 UTC: 150 bài, 48 toàn văn, 0 lỗi; 9 bài ở 2 feed lưu một lần.
- Go: 33 test `internal/news` + 25 test `cmd/server` (gồm integration PostgreSQL 17 tạm) qua; Node 12 qua; govulncheck sạch; Docker build qua.
- Chrome desktop: icon, toàn văn, ảnh CDN, 0 lỗi CSP. Mobile 375px (CDP headless): không tràn ngang. Mốc đã xem không đổi.
- Chưa gặp: emagazine, album, infographic, tường thuật trực tiếp. Chi tiết: CHANGE_REPORT.md (Lượt 5).

## Lượt 6: video (2026-10-05)

- Khảo sát bài thật mỗi nguồn: Tuổi Trẻ MP4 công khai (206, CORS `*`, không phụ thuộc Referer); VnExpress HLS bị chặn hotlink khi kiểm lại (405/406/407 nếu Referer không phải vnexpress.net); BBC embed chính thức không hiển thị trong iframe từ origin local, video `/news/videos/` có `isEmbeddingAllowed:false`.
- Chrome thật: MP4 Tuổi Trẻ trong app phát (thời gian tăng), pause, tua, fullscreen, dừng khi chuyển bài; fallback đúng khi nguồn lỗi, CSP chặn host lạ. Mobile 375px: CDP headless; chưa thử điện thoại thật.
- Go 40+ test `internal/news`, Node 19 test; govulncheck sạch; Docker build qua. Chi tiết: CHANGE_REPORT.md (Lượt 6).

## Lượt 7: import nguồn bằng URL (2026-10-05)

- Nhận diện URL không gửi request; chỉ đọc RSS đã nhận diện, qua client có allowlist/IP công khai/timeout/giới hạn kích thước.
- Nguồn thật: trang chủ/RSS/chuyên mục đã có → "đã có"; khôi phục, thêm mới (worker lấy bài ngay lượt sau), feed 404 → thất bại; feed thử đã xoá mềm lại.
- 4 request đồng thời không tạo trùng; 6 lượt/phút; 401 khi thiếu mã; URL nội bộ/giả host bị từ chối.
- Tổng Go 74 test (gồm integration PostgreSQL 17 tạm), Node 19. Chi tiết: CHANGE_REPORT.md (Lượt 7).

## Lượt 9: quốc gia của nguồn báo, đọc công khai, đăng nhập quản trị (2026-10-06)

- Sao lưu trước migration: `backups/news-before-country-auth-20261006.dump` (trước khi sửa code) và `backups/news-before-country-auth-deploy-20261006.dump` (ngay trước khi triển khai); kiểm tra đọc được bằng `pg_restore --list` (5 bảng dữ liệu).
- Migration 012/013 trên dữ liệu thật: 1371 bài / 1233 toàn văn / 23 nguồn (23 URL khác nhau) / 1514 dòng `article_feeds` trước và sau, giữ nguyên. Publishers: vnexpress=VN, bbc=GB, tuoitre=VN. Chạy lại migration (xoá dòng `schema_migrations` rồi migrate): không lỗi, không nhân bản, giữ lựa chọn của quản trị viên (integration test).
- Go: gofmt sạch, `go vet` qua; 54 test qua khi không có DB (28 integration bỏ qua); 82/82 qua với PostgreSQL 17 tạm (gồm 12 test mới về phân quyền/phiên/CSRF/CLI/quốc gia). Node 23/23 qua. `govulncheck`: 0 lỗ hổng có đường gọi; 1 khuyến cáo module (GO-2026-5932, gói `x/crypto/openpgp`, không được import, chưa có bản sửa). Docker build (vet + test) qua.
- Stack thử riêng (bản khôi phục của dump, cổng 8081, image mới, đã xoá sau khi kiểm tra):
  - Khách: đọc tin thật, lọc VN/GB/unknown/TH, kết hợp chuyên mục/nguồn/từ khoá, phân trang. Bài BBC nói về Việt Nam vẫn là GB.
  - 9 route quản trị × 4 kiểu (không cookie, Bearer cũ, header "localhost"/X-Forwarded-For, cookie giả): đều 401. 8 route cũ (`/api/sources` PATCH/DELETE/import, `/api/status`, `/api/session`…): 404. Stack chạy kèm `LOCAL_NO_AUTH=true` và `ADMIN_TOKEN`: không cấp quyền gì.
  - API công khai (`/api/sources`, `/api/articles`, chi tiết bài, `/api/categories`, `/api/countries`, `/api/admin/session` khi chưa đăng nhập): không chứa feed_url, last_error, etag, content/enrich_error, csrf, token, mật khẩu, email; `Cache-Control: no-store`.
  - CLI: chưa có tài khoản → `status` báo chưa có; `create` qua stdin; tạo lần hai bị từ chối; mật khẩu ngắn bị từ chối; DB chỉ có hash argon2id.
  - Đăng nhập: sai mật khẩu và sai tên trả cùng thông báo 401; cross-site/`text/plain` 403; đúng → cookie `HttpOnly; SameSite=Strict; Path=/; Max-Age`. PATCH thiếu/sai CSRF, Origin lạ, `Sec-Fetch-Site: cross-site` → 403; `text/plain` → 415; hợp lệ → 200. Đăng xuất → cookie bị xoá; dùng lại cookie cũ: GET/PATCH/DELETE/import đều 401.
  - Hết hạn phiên (TTL 4m, idle 2m): phiên hoạt động liên tục bị từ chối sau 250 s (quá TTL 240 s). Với idle 1m/TTL 10m: phiên không thao tác 75 s → 401. Giới hạn: 5 lần sai → lần 6 với mật khẩu đúng → 429.
  - Chrome thật (desktop, cửa sổ 1403px): khách vào thẳng bản tin, không có tab Nguồn tin/form thêm nguồn; lọc quốc gia + chuyên mục + trang 2 + từ khoá; chấm đỏ giữ nguyên khi mở chuyên mục lúc đang lọc quốc gia, chỉ mất khi mở chuyên mục không lọc; đọc bài hiện "· Vương quốc Anh". Đăng nhập sai/đúng qua form; đổi quốc gia Tuổi Trẻ → Thái Lan (bộ lọc có "Thái Lan", lọc TH ra bài Tuổi Trẻ) rồi đổi lại Việt Nam; tắt/bật nguồn; import khôi phục feed; xoá nguồn; nút "Báo lỗi qua email" chỉ chứa URL đã nhập; đăng xuất → ghi bị 401. `document.cookie` không thấy cookie phiên; storage chỉ có `news-seen-v1`.
  - Mobile 375px: **giả lập** (Chrome headless, CDP device emulation 375×812, mobile, touch, profile tạm), không phải điện thoại thật. Tin, tin lọc GB, đọc bài, đăng nhập, quản lý nguồn: không tràn ngang (sau khi sửa select trong khung quốc gia tràn 11px).
- App local (cổng 8080) sau triển khai: healthy; migration chạy một lần; khách đọc và lọc được; ma trận route quản trị như trên (401/404); `ADMIN_TOKEN` thật trong `.env` gửi dạng Bearer → 401/404. Tài khoản tạm `tmp-verify`: đăng nhập, xem nguồn/website, import trang chủ VnExpress → "exists" (không đổi dữ liệu), import thiếu CSRF 403, đăng xuất → 401; đã xoá tài khoản (0 tài khoản, 0 phiên). Log không chứa mật khẩu/cookie/token. Trình duyệt kiểm tra qua `http://localhost:8080` (origin khác), không mở `http://127.0.0.1:8080` nên trạng thái đã xem của bạn không bị đụng tới.
