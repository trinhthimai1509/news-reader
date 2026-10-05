# Báo cáo audit — Personal News Reader

> Thay đổi sau audit (truy cập local không cần mã, chuyên mục, badge bài mới): xem CHANGE_REPORT.md.

Ngày: 2026-10-05. Phạm vi: toàn bộ source (Go, SQL, HTML/CSS/JS, Dockerfile, Compose), chạy end-to-end trên máy local (macOS, Docker qua Colima) với nguồn thật.

## 1. Kết luận

**Chạy local: SẴN SÀNG để kiểm tra.** App và PostgreSQL chạy bằng Docker Compose, lấy tin thật từ VnExpress và BBC, lưu PostgreSQL, đọc qua API và giao diện web đã được kiểm tra bằng trình duyệt thật.

**Chưa sẵn sàng bàn giao khách / production.** Còn các blocker ở mục 7 (HTTPS, sao lưu, độ trễ feed VnExpress "tin mới nhất", quyền sử dụng nội dung).

Không có lỗi critical. Các lỗi high/medium tìm thấy đã được sửa và có regression test (trừ phần ghi rõ ở mục 3).

## 2. Findings đã sửa

| # | Severity | Vị trí | Tình huống / ảnh hưởng | Sửa |
|---|---|---|---|---|
| H1 | High | `go.mod`, `Dockerfile` | govulncheck: 10 lỗ hổng **có đường gọi tới** trong `golang.org/x/net` v0.35.0 (gồm parser HTML — app parse HTML không tin cậy từ nguồn), `pgx` v5.7.2, `x/text` v0.22.0. Go 1.23 đã hết hỗ trợ. | Nâng pgx v5.11.0, x/net v0.59.0, x/text v0.42.0, x/crypto v0.57.0; `go 1.26.0`; image build `golang:1.27-alpine3.24`, runtime `alpine:3.24`. govulncheck sau sửa: *No vulnerabilities found*. |
| M1 | Medium | `internal/news/feed.go` Extract | BBC: caption ảnh, danh sách "related", "Related topics", quảng cáo newsletter bị lưu như đoạn văn bài; trang video BBC bị gắn "full". VnExpress: caption ảnh, dòng link `>> ...`, bài crossword (game) bị gắn "full". Kiểm chứng trên 13 trang thật. | BBC chỉ lấy `<p>/<h2>` trong `data-block="text"`/`subheadline`, bỏ đoạn chỉ là link `/newsletters/`. VnExpress bỏ `figure/figcaption/.Image/.tplCaption`, widget `data-component`, dòng `>>`; bài có widget crossword/quiz/minigame → không phải toàn văn. Ngưỡng tối thiểu 2 đoạn và 300 ký tự. |
| M2 | Medium | `cmd/server/main.go` poll | Polling tuần tự, không có hạn thời gian theo nguồn: một nguồn chậm (20s × 11 request) có thể giữ lượt polling > 3 phút và chặn nguồn khác. | Mỗi site (adapter) chạy song song; mỗi nguồn có ngân sách 90s; hết ngân sách thì dừng, không tính là lần lỗi. |
| M3 | Medium | worker | Kiểm thử thật: VnExpress trả **HTTP 429** khi lấy nhiều trang bài liên tiếp; bài bị đánh lỗi và tốn lượt retry. | Các nguồn cùng site chạy tuần tự, nghỉ 1,5s giữa các trang bài; 429/503 → dừng lấy trang bài của site trong lượt đó, không tăng `attempts`. Sau sửa: không còn 429 trong lượt tiếp theo. |
| M4 | Medium | khử trùng URL | Link BBC có `?at_medium=RSS&at_campaign=rss`; cùng bài trên bbc.com/bbc.co.uk hoặc khác tham số tracking sẽ thành nhiều bản ghi. Feed URL khác hoa/thường hoặc có `#` tạo nguồn trùng. | `Canonical()`: bỏ fragment, `utm_*`, `at_*`, `fbclid`, `gclid`, `ocid`; host chữ thường; host bài BBC chuẩn hoá về `www.bbc.co.uk`. Kiểm tra DB thật: 0 URL còn tracking. |
| M5 | Medium | xoá nguồn | Bài thuộc nguồn đầu tiên phát hiện. Xoá nguồn A thì bài bị ẩn dù nguồn B đang bật vẫn liệt kê bài đó. | `ON CONFLICT(url) DO UPDATE SET source_id=...` chỉ khi chủ cũ đã bị xoá. Có integration test. |
| M6 | Medium | SSRF `Client()` | Bộ lọc IP bỏ sót 100.64/10 (CGNAT), 198.18/15, 240/4, TEST-NET, NAT64 `64:ff9b::/96`, 6to4 `2002::/16`, Teredo... Ảnh hưởng thấp hơn nhờ allowlist host, nhưng DNS bị đầu độc/rebinding có thể trỏ vào mạng nội bộ. | `PublicIP()` dùng `netip` + danh sách dải đặc biệt, unmap IPv4-in-IPv6. Kết nối tới đúng IP đã kiểm tra (không TOCTOU). Không dùng proxy môi trường. |
| M7 | Medium | `main()` | Chạy Go trực tiếp bind `:8080` = mọi interface (lộ ra LAN). | Mặc định `127.0.0.1:8080`; container đặt `LISTEN_ADDR=:8080` và Compose chỉ publish `127.0.0.1`. |
| M8 | Medium | migration | Chỉ một file SQL, không ghi phiên bản; thay đổi schema sau này không có cơ chế an toàn. | Runner `migrate()`: áp dụng `migrations/*.sql` theo thứ tự, một lần, trong 1 transaction có `pg_advisory_xact_lock`; bảng `schema_migrations`. Thêm `002_indexes.sql`. Test 4 tiến trình migrate đồng thời: qua. |
| L1 | Low | auth | Header `Authorization: <token>` (thiếu `Bearer `) vẫn được chấp nhận. | Bắt buộc tiền tố `Bearer `. Test. |
| L2 | Low | auth | Không giới hạn số lần thử token. | 10 lần sai / 5 phút / địa chỉ client → 429. Test. |
| L3 | Low | `GET /api/articles` | `source`, `page` sai bị bỏ qua im lặng; `%`/`_` trong từ khoá là wildcard; nút "Trang sau" vẫn bật khi trang cuối đủ 30 bài. | Trả 400 cho tham số sai; tìm kiếm literal; API trả `{items, page, has_more}` (lấy 31 dòng). |
| L4 | Low | `POST /api/sources` | Giới hạn tên 100 *byte* (tên tiếng Việt 40 ký tự có thể bị từ chối). | Đếm ký tự Unicode; thông báo lỗi nêu rõ dạng URL hợp lệ. |
| L5 | Low | ngày đăng | Không parse ngày 1 chữ số (`Mon, 5 Oct`); ngày tương lai đẩy bài lên đầu mãi. | Thêm định dạng; ngày > now+1h hoặc thiếu ngày → dùng thời điểm lấy tin. |
| L6 | Low | `web/app.js` | Lỗi không phải JSON (proxy 502, body quá lớn) hiện "SyntaxError"; đoạn đầu bài VnExpress lặp lại tóm tắt; không phân biệt "đang chờ" với "không lấy được". | Xử lý lỗi mạng/không JSON; bỏ đoạn đầu nếu trùng tóm tắt; nhãn "Toàn văn" / "Tóm tắt · đang chờ" / "Chỉ có tóm tắt" và thông báo tương ứng; trạng thái rỗng khi lọc. |
| L7 | Low | Compose/Docker | Thiếu healthcheck app; container ghi được rootfs; pool kết nối mặc định; lỗi "redirect blocked" khó hiểu. | `HEALTHCHECK`, `read_only`, `cap_drop: ALL`, `no-new-privileges`, `MaxConns=10`, `go mod verify` + `go vet` trong build; thông báo lỗi tiếng Việt nêu đích chuyển hướng. |

## 3. Findings chưa sửa / chấp nhận

| Severity | Vấn đề | Lý do / đề xuất |
|---|---|---|
| Medium (sản phẩm) | **Feed `tin-moi-nhat.rss` của VnExpress chậm ~3 giờ** khi đo từ máy này: lúc 14:22 (+07) mục mới nhất là 11:32; trang chủ đã có ID bài 5128544, feed dừng ở 5128472. Feed chuyên mục `thoi-su.rss` cùng lúc có bài 14:20. Header `cache-control: max-age=120, stale-if-error=864000`. | Nguyên nhân phía nguồn/CDN, chưa xác định có lặp lại ở môi trường khác không. Đề xuất: dùng các feed chuyên mục VnExpress khách cần (đã kiểm tra thêm nguồn `thoi-su.rss` chạy tốt). Không tự đổi seed vì là quyết định sản phẩm. |
| Low | Bài đã lấy toàn văn không được lấy lại khi nguồn sửa bài (thấy 1 bài BBC đã đổi câu mở đầu sau khi lấy). | Cần chính sách refresh (ví dụ lấy lại sau 1–6 giờ) nếu khách yêu cầu. |
| Low | `pubDate` trong RSS BBC là thời điểm *cập nhật* (ví dụ bài đăng 21:23 UTC hôm trước, RSS ghi 07:14 UTC). App hiển thị đúng giá trị RSS. | Hành vi nguồn; ghi chú cho khách. |
| Low | Token lưu `sessionStorage`: nếu có XSS thì token bị đọc. | Giảm thiểu: CSP `script-src 'self'`, không inline script, mọi nội dung chèn bằng `textContent`. Chấp nhận cho app 1 người dùng. |
| Low | Bộ giới hạn đăng nhập theo địa chỉ client; sau reverse proxy mọi request có cùng địa chỉ → kẻ tấn công có thể khoá chủ 5 phút. | Chấp nhận khi chỉ bind localhost; khi public cần giới hạn ở reverse proxy. |
| Low | Lượt polling của một site có thể vượt 2 phút nếu site đó có nhiều nguồn chậm (tối đa 90s/nguồn). Ticker bỏ qua nhịp chồng, không chạy trùng. | Đủ cho vài nguồn; nhiều nguồn hơn cần chia lịch. |
| Info | Không lấy ảnh, video, bảng đồ hoạ, hộp giải thích (BBC `visualJournalism`); link `video.vnexpress.net` và mục BBC Sounds/Video chỉ có tóm tắt. | Ngoài phạm vi bản này; UI ghi rõ "Chỉ có tóm tắt" + link gốc. |
| Info | Image Docker theo tag, chưa pin digest. | Pin digest trước khi bàn giao nếu cần build tái lập tuyệt đối. |

## 4. Thay đổi thực tế

- `go.mod`, `go.sum`: nâng dependency, `go 1.26.0`.
- `internal/news/feed.go`: `Canonical`, `PublicIP`, `ErrThrottled`, extractor theo cấu trúc trang thật, `Published(s, now)`, lỗi rõ khi nguồn trả HTML/redirect.
- `cmd/server/main.go`: auth `Bearer` + giới hạn sai token; validate tham số; phân trang `has_more`; worker theo site + ngân sách thời gian + nghỉ giữa request + xử lý 429; chuyển chủ bài khi nguồn bị xoá; runner migration; `LISTEN_ADDR`; pool config.
- `migrations/002_indexes.sql`: index `(source_id, published_at)` và index bài chờ lấy nội dung.
- `web/app.js`: xử lý lỗi, phân trang, trạng thái nội dung, bỏ đoạn trùng.
- `Dockerfile`, `compose.yaml`, `.env.example` (`APP_PORT`).
- Tests: `cmd/server/main_test.go` (7 test, 2 integration cần `TEST_DATABASE_URL`), `internal/news/feed_test.go` (14 test).
- `.env` mới được tạo với secret ngẫu nhiên 32 byte hex (quyền 600); không in ra log/báo cáo.

## 5. Kết quả kiểm tra

| Nhóm | Kết quả |
|---|---|
| `gofmt -l .` | sạch |
| `go vet ./...` | qua (Go 1.27.1 trong container) |
| `go test ./...` | 19 test qua; 2 integration test qua với PostgreSQL 17 tạm thời (container riêng, đã xoá) |
| `govulncheck ./...` (v1.8.0) | trước: 10 lỗ hổng có đường gọi; sau: không có |
| `node --check web/app.js` | qua |
| `docker compose up --build` | build (gồm vet + test) và chạy; cả 2 container healthy |
| Image runtime | user 10001, file thuộc root (không ghi được), không có `.env`, history không chứa secret |
| Secret trong log | so khớp token/mật khẩu với toàn bộ log Compose: 0 lần xuất hiện |
| DB port | không publish ra host |

Kiểm tra end-to-end (API bằng curl, UI bằng Chrome thật):

1. `/healthz` → `{"status":"ok"}` 200 (ping DB).
2. Không token / token sai / token thiếu `Bearer` → 401; UI hiện "Mã truy cập không đúng".
3. Đăng nhập bằng mã từ `.env` → vào được bản tin.
4. Tin thật được lưu: ~180 bài từ 4 nguồn; 149 toàn văn, 19 đang chờ, 10 chỉ tóm tắt (video BBC, bài VnExpress bị gỡ, crossword, bài chủ yếu là đồ hoạ).
5. Tìm kiếm (tiếng Việt có dấu, tiếng Anh), lọc nguồn, phân trang 6 trang (Trang sau tắt ở trang cuối, Trang trước tắt ở trang 1).
6. Đọc bài: tên nguồn, thời gian, toàn văn, link "Đọc bài gốc tại …" (`rel=noopener noreferrer`); bài lỗi hiện thông báo tóm tắt + link gốc.
7. Thêm `thoi-su.rss` (VnExpress) và `news/business/rss.xml` (BBC); thêm lại cùng URL khác hoa/thường → cùng id; tắt/bật; xoá → bài ẩn; thêm lại → bài hiện lại.
8. Bị từ chối (400): host ngoài allowlist, `http://`, `vnexpress.net.evil.com`, feed BBC ngoài `/news/`, IP `169.254.169.254`, adapter lạ.
9. Restart (rebuild) app: 178 bài, 6 nguồn giữ nguyên; migration không chạy lại.
10. Nguồn lỗi: feed VnExpress không tồn tại (chuyển hướng về trang chủ → bị chặn), feed BBC 404 → hiện lỗi đỏ trong "Nguồn tin", nguồn khác vẫn cập nhật, app không dừng.

Giao diện: kiểm tra bằng Chrome ở 1280px. Chrome không cho cửa sổ hẹp hơn 500px, nên mobile được kiểm tra ở 500px (đã vào layout mobile, breakpoint 650px) và ép `body` rộng 375px để đo tràn ngang: 0 phần tử tràn ở 3 màn hình (tin, nguồn, đọc bài). Chưa kiểm tra trên thiết bị di động thật.

## 6. Trạng thái nguồn

**VnExpress** — hoạt động trong môi trường này.
- GET `https://vnexpress.net/rss/tin-moi-nhat.rss`: 200 `application/xml`, 38–39 mục. (HEAD trả 406 — đây là lý do báo cáo cũ ghi "chưa xác minh"; app dùng GET.)
- 5 bài thường + 1 crossword + 1 bài đã gỡ (302 → `/404.html`) được kiểm tra tay; 3 bài đối chiếu tự động với trang gốc: tiêu đề, giờ (GMT+7 ↔ UTC), đoạn đầu/cuối khớp. Không lấy caption, link `>>`, khung bình chọn, "Xem thêm".
- Giới hạn: feed "tin mới nhất" chậm ~3 giờ (mục 3); trả 429 khi gọi dồn dập (đã xử lý); bài crossword/đồ hoạ chỉ có tóm tắt.

**BBC News (tiếng Anh)** — hoạt động.
- GET `https://feeds.bbci.co.uk/news/rss.xml`: 200, 29 mục; feed mới (mục mới nhất cách vài phút).
- 3 bài news + 1 sport + 1 video + 1 Sounds + 1 trang app được kiểm tra; 3 bài đối chiếu tự động: tiêu đề khớp `<h1>`, đoạn văn có trong trang. Không lấy caption, link liên quan, chủ đề, newsletter.
- Giới hạn: feed tổng có cả Sport/Sounds/Video; video & Sounds chỉ có tóm tắt; `pubDate` là giờ cập nhật; hộp giải thích/đồ hoạ không lấy.

**Độ trễ:** lịch kiểm tra 2 phút + tối đa ~90s xử lý mỗi nguồn. Độ trễ thực tế phụ thuộc thời điểm nguồn đưa bài vào RSS — với VnExpress "tin mới nhất" đã đo được ~3 giờ. **Chưa thể cam kết SLA 5 phút.** Feed tổng không bảo đảm có mọi bài của mọi chuyên mục.

## 7. Chạy, kiểm tra, dừng

```sh
cd ~/Documents/news-reader
docker compose ps                    # trạng thái
docker compose logs -f app           # log app
grep '^ADMIN_TOKEN=' .env | cut -d= -f2- | pbcopy   # chép mã truy cập vào clipboard
```

Mở **http://127.0.0.1:8080** (hoặc http://localhost:8080). Từ lượt 2, môi trường local này bật `LOCAL_NO_AUTH=true` nên không cần mã (xem CHANGE_REPORT.md); nếu tắt chế độ đó thì dán mã vào ô "Mã truy cập".

Dừng an toàn (giữ dữ liệu): `docker compose stop` (chạy lại: `docker compose start`) hoặc `docker compose down`. **Không** dùng `docker compose down -v` (xoá volume dữ liệu `news-reader_pgdata`).

Dữ liệu kiểm thử còn trong DB: nguồn "VnExpress Thời sự", "BBC Business" (đang bật) và hai nguồn "… lỗi (test)" (đã xoá mềm). Có thể tắt/xoá trong màn hình Nguồn tin.

Tài nguyên phụ tạo khi audit: Docker volume `news-reader-gocache` (cache Go cho test) — xoá được bằng `docker volume rm news-reader-gocache`.

## 8. Blocker trước khi bàn giao khách

1. Chọn feed VnExpress: đo lại độ trễ `tin-moi-nhat.rss` từ môi trường triển khai; nếu vẫn chậm, dùng feed chuyên mục.
2. HTTPS qua reverse proxy (Caddy/Nginx) nếu truy cập từ xa; giới hạn đăng nhập ở proxy.
3. Sao lưu/khôi phục PostgreSQL (`pg_dump` theo lịch) và thử restore.
4. Xác nhận điều khoản sử dụng nội dung toàn văn của VnExpress/BBC cho mục đích của khách.
5. Đo độ trễ và tỷ lệ toàn văn trong 24 giờ trước khi hứa thời gian cập nhật.
6. Kiểm tra trên điện thoại thật; quy trình đổi `ADMIN_TOKEN`.
