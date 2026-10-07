# Báo cáo thay đổi — truy cập local, chuyên mục, bài mới

Ngày: 2026-10-05. Tiếp nối AUDIT_REPORT.md; giữ nguyên các bản sửa bảo mật và dependency của lượt audit.

## 1. Truy cập local không cần mã

Bật bằng `LOCAL_NO_AUTH=true` (mặc định `false` = giữ xác thực bằng mã như cũ). App kiểm tra ở hai mức:

**Khi khởi động** (từ chối chạy nếu sai):
- Go chạy trực tiếp: `LISTEN_ADDR` phải là `127.0.0.1`/`::1`/`localhost`.
- Docker: trong container app buộc phải nghe `:8080`, nên app chỉ chấp nhận khi `PUBLISHED_HOST` là loopback. Compose lấy `PUBLISHED_HOST` và địa chỉ của port mapping từ **cùng** biến `APP_BIND` (mặc định `127.0.0.1`), nên hai giá trị này không lệch nhau. `APP_BIND=0.0.0.0` khiến app từ chối khởi động ở chế độ này. Gateway mặc định của container (đọc từ `/proc/net/route`, hiện là `172.19.0.1`) là địa chỉ mà kết nối từ host đi vào, nên được tin cậy.

**Mỗi request** (quyết định theo địa chỉ TCP thực, không theo header):
- Chỉ tin địa chỉ peer loopback hoặc gateway container; container khác trong cùng mạng Docker vẫn phải dùng mã.
- Từ chối nếu có `Forwarded`/`X-Forwarded-For`/`X-Forwarded-Host`/`X-Real-IP` (dấu hiệu đi qua proxy).
- Host header phải là localhost/127.0.0.1/::1 (chống DNS rebinding). Header chỉ được dùng để **từ chối**, không bao giờ để cấp quyền.
- Request ghi (POST/PATCH/DELETE): `Origin` (nếu có) phải trùng trang, `Sec-Fetch-Site` (nếu có) phải `same-origin`, POST/PATCH phải `application/json` (chống CSRF từ website khác mở trong cùng trình duyệt).
- Mã truy cập vẫn hoạt động song song. Không có endpoint trả token; `/api/session` chỉ trả `{"auth_required": true|false}`.

UI: nếu `auth_required=false` thì vào thẳng bản tin, ẩn màn hình nhập mã và nút Khoá. Nếu server sau đó trả 401 (đã tắt chế độ local) thì quay lại màn hình nhập mã.

Xác minh trên máy này trước khi bật:
- Container chỉ bind `127.0.0.1:8080`.
- Socket trên máy chỉ có `127.0.0.1:8080` (bộ chuyển cổng của Colima).
- Request tới IP LAN của máy (cổng 8080) bị từ chối kết nối.
- VM Colima không có địa chỉ mạng ra ngoài.

Kiểm tra trực tiếp sau khi bật:

| Trường hợp | Kết quả |
|---|---|
| Từ máy này, không có mã | 200 |
| Host header lạ | 401 |
| `X-Forwarded-For` | 401 |
| POST với Origin lạ | 401 |
| POST `text/plain` | 401 |
| DELETE cross-site | 401 |
| PATCH cùng origin | qua |
| Container `db` gọi `app:8080` không có mã | 401 |
| Container `db` gọi kèm mã | 200 |

Trở lại chế độ cần mã: đặt `LOCAL_NO_AUTH=false` (hoặc xoá dòng đó) trong `.env`, rồi chạy `docker compose up -d app`.

Không bật chế độ này sau reverse proxy. Proxy cấu hình không gửi header forwarding sẽ khiến mọi người dùng từ xa đi qua như request local.

## 2. Chuyên mục

Chín chuyên mục: Thời sự, Thế giới, Kinh doanh, Công nghệ, Giải trí, Thể thao, Sức khỏe, Đời sống, Khác. Bài được gán chuyên mục chỉ qua feed, không đoán từ tiêu đề.

- Mỗi feed (nguồn) có một chuyên mục, chọn khi thêm hoặc đổi trên thẻ nguồn.
- Bảng `article_feeds` ghi lại mọi feed đã liệt kê một bài. Bài lưu **một lần** theo URL chuẩn hoá và thuộc chuyên mục của mọi feed (chưa bị xoá) đã liệt kê nó. Quy tắc chủ sở hữu (`articles.source_id`) giữ nguyên như lượt audit.
- Bộ lọc nguồn khớp mọi feed đã liệt kê bài. Chuyên mục, nguồn, tìm kiếm và phân trang kết hợp được với nhau; đổi bộ lọc thì quay về trang 1.
- Feed tổng hợp (VnExpress tin mới nhất, BBC News trang chủ) thuộc Khác.

Feed đã cấu hình (mỗi feed đã kiểm tra bằng GET: 200 XML, link thuộc host hợp lệ):

| Chuyên mục | VnExpress | BBC |
|---|---|---|
| Thời sự | thoi-su.rss | — |
| Thế giới | the-gioi.rss | news/world |
| Kinh doanh | kinh-doanh.rss | news/business |
| Công nghệ | khoa-hoc-cong-nghe.rss | news/technology |
| Giải trí | giai-tri.rss | news/entertainment_and_arts |
| Thể thao | the-thao.rss | — (BBC Sport nằm ngoài allowlist `/news/`, chưa mở rộng) |
| Sức khỏe | suc-khoe.rss | news/health |
| Đời sống | gia-dinh.rss (VnExpress không có `doi-song.rss`) | — |
| Khác | tin-moi-nhat.rss | news/rss.xml |

Tổng cộng 15 feed đang bật. Không cam kết bao phủ mọi bài hoặc mọi lĩnh vực.

**Tải lên nguồn:** worker đọc các feed của mỗi site lần lượt, cách nhau 1 giây. Sau đó mỗi lượt lấy tối đa 12 trang bài mỗi site, cách nhau 1,5 giây, trong tối đa 60 giây. Gặp 429/503 thì dừng site đó trong lượt. Lượt đầu sau khi thêm feed: không có lỗi và không có 429. Hệ quả: toàn văn cho phần tồn đọng (~280 bài) được lấy dần trong khoảng 1 giờ; trong thời gian đó các bài hiển thị nhãn "Tóm tắt · đang chờ".

**Migration `003_categories.sql`:** có phiên bản, chỉ thêm (bảng/cột/index `IF NOT EXISTS`, `ON CONFLICT DO NOTHING`), chạy trong runner có advisory lock. Trên dữ liệu thật:
- 180 bài và 170 bài toàn văn cũ còn nguyên.
- 6 nguồn cũ giữ id. Nguồn đã có (`thoi-su.rss`, BBC Business) không bị nhân bản.
- Chuyên mục của nguồn cũ chỉ được gán khi URL feed đã lưu là feed chuyên mục đã biết. Các nguồn còn lại vào Khác.
- `article_feeds` cho bài cũ chỉ backfill từ feed đã lưu (`articles.source_id`). Feed khác có thể từng liệt kê bài đó nhưng không được ghi lại thì không suy diễn.
- Đã sao lưu DB bằng `pg_dump` trước khi migrate.

## 3. Badge bài mới

- **"Mới"** = bài hệ thống nhận lần đầu sau lần cuối bạn mở chuyên mục đó trên trình duyệt này. Mốc là id bài (cấp theo thứ tự nhận), không dùng `published_at`. Server cấp id trong một transaction giữ advisory lock, nên thứ tự id trùng với thứ tự commit và mốc không bao giờ bỏ sót bài commit muộn.
- Trình duyệt lưu `{chuyên mục: mốc}` trong `localStorage` (`news-seen-v1`). Lần đầu dùng, mọi chuyên mục lấy mốc hiện tại, nên lịch sử cũ không bị coi là mới. Tải lại trang vẫn giữ mốc. Không đồng bộ giữa thiết bị hay trình duyệt.
- Mở một chuyên mục (bấm chip, đổi trang, "Tải lại") mà danh sách tải **thành công** và **không** lọc theo nguồn/từ khoá thì mốc của chuyên mục đó được đặt bằng `cursor` trả về cùng danh sách. Danh sách không bao giờ chứa bài có id lớn hơn cursor đó.
- Không đánh dấu khi: request lỗi, refresh nền 2 phút, ở màn hình Tất cả, hoặc danh sách đang lọc theo nguồn/từ khoá.
- Server đếm (`GET /api/categories?seen=slug:id,...`) bằng một truy vấn SQL; trình duyệt không tải lịch sử để đếm. Chỉ đếm bài đọc được (chủ không bị xoá) và thuộc chuyên mục qua feed chưa bị xoá. Một bài nằm ở hai chuyên mục được đếm ở cả hai.
- UI: chip "Thế giới · 1 mới", ẩn khi bằng 0. Thẻ bài có nhãn "Mới" theo mốc trước lần mở hiện tại.

Kiểm chứng với dữ liệu thật trong Chrome:
1. Lần đầu: mốc 4357, không có badge.
2. Lượt polling nhận bài id 5341 (đăng 07:51 UTC, nhận 07:59 UTC): xuất hiện "Thế giới · 1 mới".
3. Tải lại trang và refresh nền: mốc vẫn 4357.
4. Bấm Thế giới: mốc thành 5341, badge biến mất.
5. Request lỗi (chuyên mục không hợp lệ): hiện thông báo lỗi, mốc không đổi.

## 4. Sửa phát sinh khi kiểm tra

- **File tĩnh không có Cache-Control** (Low, đã sửa): Chrome dùng `index.html` cũ trong cache cùng `app.js` mới, làm trang mở ra màn hình nhập mã. Nay file tĩnh trả `Cache-Control: no-cache` (kiểm tra lại bằng 304). Có test.
- Chip đang chọn bị khuất trên màn hình hẹp: tự cuộn vào tầm nhìn.

## 5. Kiểm tra

| Nhóm | Kết quả |
|---|---|
| gofmt, go vet | sạch / qua |
| Go unit | 25 qua (10 `cmd/server`, 15 `internal/news`) |
| Go integration (PostgreSQL 17 tạm, đã xoá) | 7 qua: migration đồng thời; migration giữ dữ liệu cũ + backfill có bằng chứng + không nhân bản feed; mapping và dedup nhiều feed; chuyển chủ khi nguồn bị xoá; đếm bài mới (bài đến muộn, bài sau mốc, bài thuộc 2 chuyên mục, chủ bị xoá); cursor của danh sách; kết hợp chuyên mục/nguồn/tìm kiếm + phân trang |
| Node (`node --test tests/`) | 7 qua: mốc lần đầu, giữ sau reload, request lỗi, refresh nền/Tất cả/đang lọc không đánh dấu, bài sau mốc, mốc không lùi, storage hỏng/bị chặn |
| govulncheck | không có lỗ hổng |
| `node --check` | app.js, seen.js qua |
| Docker build (gồm vet + test) | qua; app và db healthy |
| Browser desktop (Chrome thật, 1280px) | vào thẳng không cần mã; thanh chuyên mục; badge; mở chuyên mục; lọc kết hợp; trạng thái rỗng/lỗi; thêm/sửa/xoá feed kèm chuyên mục |
| Mobile 375px | device emulation qua Chrome DevTools Protocol (Chrome headless, profile tạm, 375×812, mobile, touch): 4 màn hình (tin, chuyên mục, đọc bài, nguồn) không tràn ngang; thanh chuyên mục cuộn ngang bên trong |
| Secret | token không có trong log hay `web/`; không có endpoint trả token |

## 6. Giới hạn còn lại

- Trạng thái đã xem chỉ nằm trên từng trình duyệt; xoá dữ liệu trang web sẽ đặt lại mốc (không đánh dấu toàn bộ lịch sử là mới).
- Bài cũ (trước migration) chỉ có chuyên mục của feed đầu tiên đã lưu, nên phần lớn bài cũ nằm trong Khác.
- Chuyên mục Thời sự, Thể thao, Đời sống chỉ có feed VnExpress; BBC Sport cần mở rộng allowlist nếu muốn.
- Đổi chuyên mục của một feed sẽ đổi chuyên mục của mọi bài feed đó đã liệt kê (vì mapping theo feed).
- Chế độ local không cần mã chỉ dành cho máy cá nhân; không dùng sau reverse proxy hoặc khi publish port ra LAN.
- Các blocker trước khi bàn giao khách trong AUDIT_REPORT.md (HTTPS, backup, quyền nội dung, độ trễ feed) vẫn còn.

---

# Lượt 3 — tác giả, hình ảnh, xoá từ khoá tìm kiếm (2026-10-05)

## 1. Khảo sát cấu trúc thật (trước khi sửa)

| | Tác giả | Ảnh trong bài | Caption / credit |
|---|---|---|---|
| VnExpress | `meta author` và JSON-LD chỉ là tổ chức "VnExpress" → **không dùng**. Tác giả là dòng ký tên cuối bài, căn phải (`<p style="text-align:right">` hoặc `<p align="right">`, tên trong `<strong>/<b>`, kèm "(theo …)", "(Ảnh: …)", "A - B", "X tổng hợp") | `figure.tplCaption` (lazy: `src` là ảnh mờ/GIF, URL thật ở `data-src`/`data-srcset`, kích thước ở `meta itemprop`), `figure` dạng longform, slideshow `item_slide_show` (ảnh ở `data-src`; lời ảnh lặp hai lần, một bản trong lớp phủ) và video nhúng (chỉ có poster); host `*.vnecdn.net` | "Chú thích. Ảnh: X" trong `figcaption`/`p.Image` |
| BBC | Khối `data-block="byline"`: `single-byline` ("By" + tên, có thể là link, + chức danh + avatar) hoặc `multi-byline` ("A and B, BBC News Investigations"); có bài không có byline; không có JSON-LD author | Chỉ trong `data-block="image"`; ảnh có `src` thật + `srcset`; ảnh trong khối links/media/byline là thumbnail/poster/avatar; cuối bài có banner newsletter (PNG, tỷ lệ ≥ 6:1, alt "… banner", không caption); host `ichef.bbci.co.uk`; một số ảnh đầu bài chỉ có trong `og:image` (BBC render bằng JS) | caption `<p>` trong `figcaption` (bỏ chữ ẩn "Image caption,"), credit `span[role=text]` (bỏ "Image source,") |

## 2. Đã sửa

- **Tác giả:**
  - Trích từ dòng ký tên (VnExpress) hoặc byline (BBC); hỗ trợ nhiều tác giả.
  - Bỏ: tên tổ chức, chức danh, "theo Reuters", người được nhắc trong bài, nhãn "Thiết kế/Ảnh/Video".
  - Dòng ký tên không còn bị lẫn vào đoạn văn.
  - Không có tác giả → để trống và UI ẩn dòng "Tác giả".
- **Nội dung dạng blocks:** đoạn văn và ảnh theo đúng thứ tự trong bài. Mỗi ảnh có `src`, `srcset`, kích thước, alt (chép nguyên từ nguồn, không tự tạo), caption và credit tách riêng khỏi đoạn văn.
- **Ảnh bị loại:** quảng cáo/widget (`data-component`), thumbnail bài liên quan, avatar tác giả, banner newsletter, poster video, URL không phải HTTPS hoặc ngoài allowlist, `data:`/`javascript:`.
- **Ảnh đại diện:**
  - BBC: lấy `og:image`, đổi sang bản `standard` (bản `branded` có gắn logo BBC); bỏ qua nếu cùng file đã có trong thân bài.
  - VnExpress: `og:image` là bản crop riêng, không so trùng được, nên chỉ dùng khi thân bài không có ảnh.
  - Bài chưa lấy được toàn văn dùng ảnh RSS (`enclosure` / `media:thumbnail`).
- **Sửa lỗi có sẵn:** bài VnExpress dạng ảnh (slideshow) bị lưu trùng mọi đoạn văn (ví dụ 50 đoạn, chỉ 26 đoạn khác nhau). Nay mỗi đoạn chỉ còn một lần.
- **Lỗi nghiêm trọng phát hiện khi chạy thật, đã sửa:**
  - Mục slideshow VnExpress không có khung ảnh gây nil-pointer panic trong worker. App crash và Docker restart 9 lần trong khoảng 5 phút; trong lúc đó UI báo "Không kết nối được máy chủ".
  - Dữ liệu không mất (mỗi lệnh ghi DB là một câu độc lập).
  - Sửa: xử lý đúng cấu trúc đó, và thêm `recover` để một trang lạ chỉ làm hỏng bài đó, không làm chết app.
  - Có regression test, gồm một test đảm bảo extractor không bao giờ panic.
  - Sau khi sửa, theo dõi nhiều lượt: 0 panic, 0 restart.
- **Ô tìm kiếm:**
  - Bấm dấu X native, Backspace/Delete về rỗng, chọn hết rồi xoá, hoặc chỉ còn khoảng trắng → tải lại với q rỗng ở trang 1, giữ chuyên mục và nguồn.
  - Enter vẫn tìm kiếm như cũ. Các sự kiện input, search và submit từ cùng một thao tác chỉ tạo một request. Không có live search khi gõ.
  - Response cũ về muộn không ghi đè kết quả mới (`SearchBox.latest`).
  - Xoá từ khoá không đánh dấu chuyên mục đã xem.

## 3. Cách lấy và hiển thị ảnh

- **Hiển thị trực tiếp từ CDN của nguồn, không tải ảnh qua backend, không có proxy URL.** Không tăng bề mặt SSRF và không tốn băng thông máy chủ.
- CSP `img-src 'self' https://ichef.bbci.co.uk https://*.vnecdn.net`: chỉ hai CDN đã xác minh từ trang thật, không dùng wildcard toàn cục, không `data:`. URL ảnh được kiểm tra ở backend (HTTPS, cổng 443, không userinfo, đúng host) trước khi lưu, và kiểm tra lại ở frontend.
- `Referrer-Policy: no-referrer` (đã có) và `referrerpolicy="no-referrer"` trên ảnh: CDN không nhận URL trang đọc.
- Ảnh có `width`/`height` để giữ tỷ lệ, co theo bề rộng màn hình, `srcset`/`sizes` để chọn kích thước phù hợp. Ảnh đầu tiên tải ngay, các ảnh sau dùng `loading="lazy"`.
- Ảnh lỗi → khung "Không tải được ảnh từ nguồn."; caption, credit và nội dung giữ nguyên.
- **Giới hạn:**
  - Trình duyệt kết nối trực tiếp tới CDN BBC/VnExpress, nên CDN thấy IP người đọc.
  - Ảnh phụ thuộc CDN còn phục vụ; không lưu bản sao, nên ảnh mất ở nguồn thì mất ở app.
  - Nếu nguồn đổi CDN, cần cập nhật allowlist và CSP.

## 4. Backfill cho bài đã lưu

- Migration `004_article_media.sql` (có phiên bản, chỉ thêm cột):
  - `authors`, `blocks`, `lead_image`, `extract_version`, và trạng thái backfill riêng (`enrich_attempts`, `enrich_next`, `enrich_error`).
  - Bài cũ giữ nguyên `paragraphs`; API trả blocks dạng đoạn văn khi `blocks` còn trống.
  - Trước khi chạy: 696 bài, 524 toàn văn. Sau khi chạy: 696 bài, không bài nào mất. Đã `pg_dump` trước.
- Bài toàn văn có `extract_version` < 3 được đọc lại **một lần**, chỉ dùng lượt trang còn dư sau bài mới. Tối đa 6 bài/site/lượt trong giới hạn 12 trang/site/lượt, vẫn giữ khoảng nghỉ 1,5 giây và xử lý 429.
- Backfill lỗi (bài bị gỡ, cấu trúc lạ, lỗi mạng) **không đụng nội dung đang lưu**; chỉ ghi `enrich_error` và thử lại sau 1 giờ rồi 2 giờ, tối đa 3 lần.
- Tiến độ hiện trong màn hình Nguồn tin và `GET /api/status`.
- Lúc báo cáo (08:52 UTC): **54 bài xong, 522 đang chờ, 0 lỗi.**
  - Bài VnExpress đang ưu tiên 91 bài mới chưa có toàn văn, nên backfill VnExpress bắt đầu sau khoảng 16 phút.
  - Ước tính hoàn tất: khoảng 45 phút cho BBC, khoảng 2 giờ cho VnExpress.
- Bài chưa backfill hiển thị ghi chú "Ảnh và tác giả của bài này đang chờ được bổ sung". Không hiển thị như thể bài không có ảnh.

## 5. Đối chiếu bài thật

- **Khảo sát bằng extractor:**
  - VnExpress, 11 trang: bài thường, phỏng vấn longform, slideshow 12 ảnh, bài có video nhúng, bài 2 tác giả.
  - BBC, 6 trang: 1 tác giả, 2 tác giả, không byline, bài sport, trang video.
- **Đối chiếu tự động dữ liệu đã lưu trong DB với trang gốc tải mới** (4 bài VnExpress + 4 bài BBC):
  - Tác giả có trên trang: 8/8 bài.
  - URL ảnh có trong trang: 14/14.
  - Caption có trong trang: 11/11.
  - Thứ tự đoạn/ảnh khớp vị trí trong HTML gốc: 130/133 khối. 3 đoạn không định vị được bằng chuỗi thô vì có thẻ inline chen giữa chữ.
- **Trên Chrome thật:**
  - Ảnh của 4 bài tải được từ CDN (`naturalWidth` > 0), 0 vi phạm CSP.
  - Bài không có byline ẩn dòng tác giả.
  - Ảnh lỗi (URL 404 thật trên CDN BBC) chỉ hiện khung thông báo, nội dung còn nguyên.

## 6. Kiểm tra

| Nhóm | Kết quả |
|---|---|
| gofmt, go vet, govulncheck | sạch / qua / không có lỗ hổng |
| Go unit (`internal/news`) | 23 qua: có/không/nhiều tác giả; lazy-load, `srcset`, URL tương đối và protocol-relative, caption/credit, URL không hợp lệ, host lạ, ảnh liên quan/avatar/banner/poster video; slideshow không trùng và không panic; ảnh đại diện không lặp; ảnh RSS |
| Go server (unit + integration với PostgreSQL 17 tạm) | 21 qua: bài cũ vẫn đọc được; backfill lỗi giữ nội dung và đúng số đếm `/api/status`; fetch lỗi giữ ảnh RSS; CSP chỉ hai CDN; migration 004 giữ dữ liệu |
| Node (`node --test tests/`) | 12 qua: xoá bằng X / bàn phím / khoảng trắng, một request cho mỗi thao tác, Enter, response cũ không ghi đè, cùng các test "đã xem" |
| Chrome thật (desktop) | bấm X native, Cmd+A+Delete, Backspace, khoảng trắng: mỗi cách đúng 1 request q rỗng, trang 1, giữ chuyên mục/nguồn, không đánh dấu đã xem; Enter tìm bình thường |
| Mobile 375px (device emulation qua CDP) | trang tin và 2 bài có ảnh: không tràn ngang; ảnh 343px giữ tỷ lệ |
| Docker build (gồm vet + test) | qua; app healthy, 0 restart sau khi sửa |

Trạng thái "đã xem" trong Chrome của bạn được lưu trước khi test và khôi phục sau đó.

## 7. Giới hạn còn lại

- Tác giả VnExpress dựa vào dòng ký tên cuối bài. Bài không có dòng này (một số bài dịch, bài PR) sẽ không có tác giả. Nhãn ghép lạ có thể tách tên chưa chuẩn.
- Không hiển thị video, ảnh đồ hoạ tương tác hay bảng. Poster video bị loại có chủ đích.
- Ảnh đại diện VnExpress chỉ hiện khi thân bài không có ảnh.
- VnExpress đôi khi đổi slug của cùng một bài (thấy bài id 5128310 có 2 URL), nên có thể có 2 bản ghi. Chưa xử lý trong lượt này; có thể khử trùng theo mã bài cuối URL.
- Backfill còn chạy tiếp vài giờ, theo nhịp chậm có chủ đích.

---

# Lượt 4 — chấm đỏ bài mới, icon nguồn, khử trùng bài VnExpress đổi slug (2026-10-05)

Các bản sửa của Lượt 3 (tác giả, ảnh, tìm kiếm, panic worker) giữ nguyên; test và kiểm tra thật bên dưới xác nhận chúng vẫn hoạt động.

## 1. Bài mới: chỉ còn chấm đỏ

- Bỏ hẳn dòng giải thích dưới thanh chuyên mục (`#categories-note` đã xoá khỏi HTML/JS), không thay bằng dòng khác.
- Chip chuyên mục: thay "N mới" bằng chấm đỏ 7px, đặt tuyệt đối trong phần padding bên phải nên chip không đổi bề rộng. Bỏ viền đỏ `has-new`; kiểu chip đang chọn giữ nguyên.
- Thẻ bài: thay nhãn "Mới" bằng chấm đỏ 7px cạnh tiêu đề. Nhãn "Toàn văn"/"Tóm tắt…" và ghi chú bổ sung nội dung giữ nguyên.
- Không nhấp nháy (`animation: none`), không hiện số. Screen reader đọc "Có bài mới" (chip) / "Bài mới" (thẻ) qua `.sr-only`.
- Logic không đổi: `seen.js`, mốc đã xem, refresh nền, xử lý lỗi. Backend vẫn trả `new_count`; UI chỉ dùng để biết có/không.

## 2. Icon nguồn

- Lấy từ trang chính thức ngày 2026-10-05 (GET trang chủ, đọc `<link rel="icon">`):
  - VnExpress: `https://s.vnecdn.net/vnexpress/restruct/images/favicon.ico` (48×48, chữ "E"), đổi sang PNG bằng `sips`.
  - BBC: `favicon-32` của `https://www.bbc.co.uk/news` (32×32, biểu tượng BBC News).
- Lưu local: `web/icons/vnexpress.png`, `web/icons/bbc.png`. Khi đọc không có request ra bên thứ ba; CSP không đổi (`img-src 'self'` đã đủ). Không dùng dịch vụ favicon, không proxy, không thêm thư viện.
- Mapping theo adapter (API bài viết nay trả thêm `adapter`): mọi feed VnExpress dùng một icon, mọi feed BBC dùng một icon. Hiển thị 16px (18px ở thẻ nguồn), `alt=""` vì tên nguồn vẫn hiện đầy đủ ngay cạnh.
- Ảnh lỗi hoặc adapter lạ → ô chữ "VN"/"BBC". Logo ứng dụng không đổi.
- Hiện ở thẻ bài, trang đọc bài (dòng "Nguồn:") và thẻ quản lý nguồn.

## 3. Bài VnExpress trùng khi đổi slug

**Xác minh:**
- Dữ liệu thật có 2 nhóm cùng mã bài: 5128310 (id 2 và 3469) và 5128557 (id 5341 và 18289). Mỗi cặp có cùng `published_at`; tiêu đề đổi theo slug.
- GET slug cũ của cả hai bài, và cả một slug bịa (`abc-5128557.html`), đều trả 301 về slug hiện tại → VnExpress định tuyến theo mã số.

**Quy tắc định danh** (`news.ArticleKey`):
- Chỉ áp dụng cho URL `https://vnexpress.net/<slug>-<6–10 chữ số>.html`: một đoạn path, slug chữ thường, không query.
- Định danh là `vnexpress:<mã>`, lưu ở cột mới `articles.ident` (UNIQUE).
- URL khác (trang `…-tong-thuat.html`, path nhiều cấp, có query, BBC…) có `ident` NULL và vẫn khử trùng bằng URL chuẩn hoá như trước. Cách bỏ tracking của BBC giữ nguyên.

**Khi nhận bài:**
- Cùng `ident` thì không tạo bản ghi và không cấp id mới, nên badge không tăng.
- Slug mới → cập nhật `url` và `title`, URL cũ chuyển vào `former_urls`.
- Feed chậm vẫn liệt kê slug cũ → nhận ra qua `former_urls` và không đổi link ngược lại.
- Feed mới liệt kê bài thì vẫn được ghi vào `article_feeds`.

**Hợp nhất bản ghi cũ** (`mergeDuplicates`, chạy mỗi lần khởi động sau migration):
- Giữ advisory lock của ingest; chạy lại an toàn (khi mọi bản ghi đã có `ident` thì không làm gì).
- Bản ghi giữ lại là id nhỏ nhất, nên mốc đã xem trên trình duyệt không đổi nghĩa.
- Gộp `article_feeds` (lấy `first_seen` sớm nhất). Link và tiêu đề lấy từ bản nhận sau cùng; mọi URL khác vào `former_urls`.
- Nội dung lấy **nguyên khối** từ bản tốt nhất (ưu tiên toàn văn, rồi extractor mới hơn, rồi nhiều block hơn). Không ghép block của hai bản.
- Tác giả và ảnh đại diện chỉ lấy từ bản khác khi bản tốt nhất không có.
- Nếu chủ bài đã bị xoá thì chuyển sang chủ còn hoạt động. Bản trùng bị xoá; quan hệ của nó xoá theo cascade sau khi đã gộp.
- Nhóm có `published_at` khác nhau được coi là chưa đủ bằng chứng: giữ nguyên và ghi vào log.

**Kết quả trên dữ liệu thật:**
- Sao lưu trước: `backups/news-before-merge-20261005.dump`, và `…-deploy.dump` ngay trước khi deploy. Cả hai bằng `pg_dump -Fc`; thư mục `backups/` đã được loại khỏi image Docker.
- Chạy thử trên bản khôi phục (PostgreSQL tạm): 2 nhóm, hợp nhất 2; lần chạy thứ hai không còn gì để làm; 0 quan hệ mồ côi.
- Live, log khi khởi động: `2 groups found, 2 merged, 2 rows removed, 0 kept apart`.
  - Trước: 705 bài / 667 toàn văn. Sau: 703 bài + bài mới nhận về, 665 toàn văn (2 bản trùng bị xoá đều là toàn văn).
  - 0 `article_feeds` mồ côi; 0 `ident` trùng.
- Bài 5341 giữ id, nay có link và tiêu đề mới ("…dịu giọng với Trung Quốc…") và thuộc Thế giới.
- Bài 2 giữ id, link và tiêu đề "Thủ môn Vozinha…", thuộc Khác và Thể thao. Nội dung của bài này là bản extractor cũ; backfill sẽ đọc lại một lần như mọi bài khác.
- Số bài mới theo mốc đã xem trước/sau deploy: giống nhau, trừ +1 ở Khác do một bài thật mới về trong lượt polling đầu.

## 4. Kiểm tra

| Nhóm | Kết quả |
|---|---|
| gofmt, go vet, govulncheck | sạch / qua / không có lỗ hổng |
| Go `internal/news` | 25 qua (thêm: định danh VnExpress gồm cặp URL thật; URL không nhận diện được và BBC giữ fallback) |
| Go `cmd/server` (unit + integration PostgreSQL 17 tạm, đã xoá) | 24 qua. Thêm: đổi slug giữ 1 bài, không tính là mới, feed chậm với slug cũ không đổi link; hợp nhất giữ nội dung tốt nhất nguyên khối, gộp chuyên mục, không mồ côi, nhóm thiếu bằng chứng giữ nguyên, chạy lại không đổi; icon phục vụ local `image/png` |
| Node (`node --test tests/`) | 12 qua (không đổi) |
| Docker build (gồm vet + test) | qua; app healthy, 0 restart |
| Chrome desktop 1280px | dòng giải thích đã mất; chấm 7×7px, không animation; không còn `.count`/"Mới"/viền đỏ; icon 16px tải từ `/icons/`; mở chuyên mục làm mất chấm của chuyên mục đó; refresh nền không xoá chấm còn lại; trang đọc bài: icon, link/tiêu đề mới sau hợp nhất, tác giả, ảnh tải được, caption tách khỏi đoạn văn; thẻ nguồn: 9 feed VnExpress cùng một icon, 6 feed BBC cùng một icon; tìm kiếm: Enter, khoảng trắng, xoá + sự kiện `search` → mỗi thao tác 1 request, q rỗng ở trang 1, không đổi mốc đã xem |
| Mobile 375px (Chrome headless qua CDP, profile tạm, mobile + touch) | tin, đọc bài, nguồn: `scrollWidth` = 375 (không tràn ngang); thanh chuyên mục cuộn ngang bên trong (971/343px); chấm và icon hiển thị |
| Truy cập local | không mã → 200; Host lạ, `X-Forwarded-For`, POST `text/plain` → 401; CSP không đổi |
| Worker | sau deploy vẫn nhận bài mới và backfill (done 319, pending 348, failed 0); không panic, không lỗi trong log; xử lý 429 không đổi |

**Trạng thái đã xem:**
- Deploy không đụng tới `localStorage`. Mốc ngay sau deploy trùng với bản ghi trước deploy.
- Trong lúc kiểm tra, mốc của 7 chuyên mục được nâng lên 52751. Đây là đúng hành vi "mở chuyên mục"; có vẻ do có người bấm chip trong cùng cửa sổ Chrome lúc đó (kích thước cửa sổ đổi và có tab mới mở giữa các bước của tôi).
- Tôi không ghi đè. Bản ghi mốc trước khi kiểm tra: `thoi-su/cong-nghe/giai-tri/the-thao/suc-khoe/doi-song/khac: 5341`, `the-gioi/kinh-doanh: 43487`.

## 5. Giới hạn còn lại

- Định danh ổn định chỉ áp dụng cho dạng URL VnExpress đã xác minh. Trang tường thuật trực tiếp (`…-tong-thuat.html`) và bài BBC đổi URL vẫn khử trùng theo URL.
- Khi đổi slug chỉ cập nhật link và tiêu đề; tóm tắt và toàn văn đã lưu không được lấy lại.
- Nhóm trùng có giờ đăng khác nhau sẽ không tự hợp nhất (hiện có 0 nhóm như vậy).
- Icon là bản chụp ngày 2026-10-05. Nếu nguồn đổi nhận diện thì thay file trong `web/icons/`.
- Các giới hạn của Lượt 2–3 vẫn còn.

---

# Lượt 5 — thử nguồn Tuổi Trẻ Online (2026-10-05)

Phạm vi: chỉ Tuổi Trẻ Online (`tuoitre.vn`), 3 feed. Không mở rộng sang `/nld/`, subdomain hay website khác. Các bản sửa trước (tác giả/ảnh/search, worker, bảo mật, chuyên mục, chấm đỏ, khử trùng VnExpress, trạng thái đã xem) giữ nguyên.

## 1. Khảo sát nguồn (từ máy này, GET, User-Agent `PersonalNewsReader/0.1`)

**Danh sách RSS chính thức** (`https://tuoitre.vn/rss.htm`): 200, liên kết tới 19 feed dạng `https://tuoitre.vn/<mục>.rss`.

**Ba feed dùng** (URL lấy từ trang danh sách):

| Feed | Kết quả GET | Chuyên mục app |
|---|---|---|
| `https://tuoitre.vn/thoi-su.rss` | 200 `text/xml`, 50 item | Thời sự |
| `https://tuoitre.vn/the-gioi.rss` | 200 `text/xml`, 50 item | Thế giới |
| `https://tuoitre.vn/kinh-doanh.rss` | 200 `text/xml`, 50 item | Kinh doanh |

- URL `/rss/thoi-su.rss` cũng trả 200 với cùng danh sách bài; allowlist chấp nhận cả hai dạng, nhưng feed được thêm dùng URL của trang chính thức.
- Feed không trả ETag/Last-Modified, nên mỗi lượt đọc lại toàn bộ (khoảng 40 KB/feed).
- `pubDate` không theo RFC: `10/5/2026 3:42:00 PM`, với U+202F trước AM/PM. Đối chiếu với `article:published_time` của trang bài (`15:42+07:00`) xác nhận đây là giờ Việt Nam.
- `<author>` trong RSS có ký tự "⭐" và không được dùng.

**Trang bài:**
- Khảo sát 9 trang: 3 Thời sự (một bài loạt nhiều ảnh, một bài clip), 3 Thế giới, 2 Kinh doanh, 1 trang `/video/`.
- Thân bài là phần tử `data-role="content"` (class `detail-content`). Phần tử `content fck` trên trang là khung popup rỗng.
- Trong thân bài có:
  - `figure type="Photo"`: `src` là bản 730px, `data-original` là file gốc, `w`/`h` là kích thước; caption trong `figcaption.PhotoCMS_Caption` dạng "… - Ảnh: X";
  - hộp `type="content"` chứa chữ của bài;
  - hộp bài liên quan `RelatedNewsBox`/`RelatedOneNews` và nút "Đọc tiếp".
- Byline: `div.detail-author[data-role=author]`, mỗi tác giả một `.author-item-name a`. Bài nhiều tác giả có class `moreauthor`. `meta author` chỉ là "TUOI TRE ONLINE".
- Trang video có `og:type=website`, không có đoạn văn, và byline chứa cả "MEDIA".
- Byline đôi khi dùng Unicode dạng tách (NFD), ví dụ "THA&#x301;I".
- Mã bài:
  - URL bài có dạng `https://tuoitre.vn/<slug>-<15–20 chữ số>.htm`.
  - GET slug sai (`abc-100261005152148723.htm`) trả 301 về slug đúng, nên mã bài là định danh ổn định.
  - Chỉ có mã, không có slug → 301 về trang chủ.
  - Trang `/video/` dùng mã ngắn hơn, không khớp mẫu.
- Ảnh trong thân bài chỉ thấy trên `cdn2.tuoitre.vn`. Không thêm các host khác thấy trên trang (`static-tuoitre.tuoitre.vn`, `static.mediacdn.vn`).
- Icon: lấy từ `<link rel="icon" sizes="32x32">` của trang chủ (`static-tuoitre.tuoitre.vn/zoom/32_32/tuoitre/images/tt-favicon.png`), lưu local ở `web/icons/tuoitre.png`.

## 2. Đã làm

- **Adapter `tuoitre` riêng** (`internal/news/tuoitre.go`), không dùng parser VnExpress. Chỉ dùng chung các helper (kiểm tra URL ảnh, đọc text, og:image, ngưỡng toàn văn).
  - Thân bài: đoạn `p`, tiêu đề phụ `h2`/`h3`, ảnh `Photo` theo đúng thứ tự, kèm chữ trong hộp `content`.
  - Bỏ: bài liên quan, widget có `type` khác (video, bình chọn…), "Đọc tiếp", bình luận, form, avatar.
  - Caption và credit tách riêng ("- Ảnh: X"). "Ảnh cắt từ clip" để nguyên trong caption.
  - Ảnh dùng bản 730px, kèm `srcset` tới file gốc. Alt chép nguyên từ nguồn.
  - Tác giả: từ byline, hỗ trợ nhiều người. Bỏ "TUOI TRE ONLINE", "TUỔI TRẺ ONLINE", "TTO", "MEDIA" và "và N tác giả khác".
  - Văn bản chuẩn hoá NFC (tiêu đề, tóm tắt, tác giả, đoạn, caption). Bỏ khoảng trắng thừa trước dấu câu sau link nội tuyến.
  - Không gắn "Toàn văn" cho trang `/video/`, `og:type=website`, trang không có thân bài hoặc quá ít chữ (ngưỡng hiện có). Các trang này giữ tóm tắt RSS và link gốc.
  - Panic trong parser vẫn chỉ làm hỏng bài đó (`recover` có sẵn).
- **URL/feed:**
  - Feed: chỉ host `tuoitre.vn`, path `/<mục>.rss` hoặc `/rss/<mục>.rss`.
  - Bài: chỉ host `tuoitre.vn`, đuôi `.htm`, không thuộc `/nld/`. Redirect ra ngoài phạm vi này bị từ chối như các adapter khác.
- **Định danh:** `tuoitre:<mã>` cho dạng URL đã xác minh. Bài thuộc nhiều feed hoặc đổi slug vẫn là một bản ghi. `mergeDuplicates` nay xử lý cả VnExpress và Tuổi Trẻ.
- **Giờ đăng:** `news.PublishedIn` đọc định dạng Tuổi Trẻ theo UTC+7. Ngày ở tương lai bị chặn như các nguồn khác. Định dạng này không áp dụng cho adapter khác.
- **Ảnh:**
  - Allowlist backend và frontend thêm đúng `cdn2.tuoitre.vn`.
  - CSP `img-src` thêm `https://cdn2.tuoitre.vn`, không dùng wildcard.
  - Ảnh RSS (`enclosure`) cùng host.
- **Migration:**
  - `006_tuoitre.sql`: mở ràng buộc `adapter` và thêm 3 feed (`ON CONFLICT DO NOTHING`). Chạy lại không tạo feed trùng; có test.
  - `007_tuoitre_nfc.sql`: chuẩn hoá NFC cho các bài Tuổi Trẻ đã lưu trước khi có bước chuẩn hoá, chỉ đổi dòng chưa ở dạng NFC.
  - Sao lưu `pg_dump` trước mỗi lần deploy (`backups/news-before-tuoitre-20261005.dump`, `…-before-nfc-…`).
- **UI:**
  - "Tuổi Trẻ" có trong ô chọn nguồn ở màn hình Nguồn tin.
  - Icon `tt` ở thẻ bài, trang đọc và thẻ nguồn; fallback "TT" nếu ảnh lỗi.
  - Chấm đỏ báo bài mới giữ nguyên.
- **Worker:**
  - Không đổi lịch hoặc giới hạn. Mỗi site chạy song song với ngân sách riêng: feed cách 1 giây, tối đa 30 giây/feed, tối đa 12 trang bài/lượt cách 1,5 giây trong 60 giây, dừng site khi gặp 429/503.
  - Tuổi Trẻ thêm tối đa 3 GET feed và 12 GET trang bài mỗi 2 phút, không lấy bớt lượt của VnExpress/BBC.

## 3. Kết quả end-to-end (RSS → PostgreSQL → API → browser)

Lúc 09:50 UTC, khoảng 6 phút sau khi bật:
- Cả 3 feed lấy thành công, không lỗi.
- Liên kết theo feed: Thời sự 55, Thế giới 53, Kinh doanh 51. Có **150 bài** khác nhau; 9 bài nằm ở 2 feed và được lưu một lần, có 2 chuyên mục.
- **48 bài toàn văn**, 102 đang chờ (được lấy dần), 0 bài lỗi. 150/150 bài có `ident`, 0 bài có ngày ở tương lai.
- Trong 48 bài toàn văn: 47 có tác giả, 3 có nhiều tác giả, 48 có ảnh trong thân bài, ảnh chỉ từ `cdn2.tuoitre.vn`.
  - Bài không có tác giả: trang gốc không có khối byline (đã kiểm tra), nên để trống.
- Lúc 36 bài toàn văn: 102 ảnh, 85 ảnh có caption.

**Đối chiếu tự động** DB với trang gốc tải mới:
- 4 bài Thời sự: đoạn văn 54/54, ảnh 10/10, caption 10/10, ảnh đúng thứ tự, 0 caption lọt vào đoạn văn.
- 1 bài mỗi chuyên mục (Thời sự, Thế giới, Kinh doanh):
  - tiêu đề, sapo = tóm tắt và giờ đăng khớp `article:published_time` ở cả 3 bài;
  - tác giả khớp byline;
  - đoạn văn 38/38, ảnh 5/5 (bằng số `figure type=Photo` trên trang), caption 5/5;
  - credit REUTERS/AFP/tên phóng viên đúng; không lẫn "ĐỌC NGAY", "Đọc tiếp", bình luận hay "TUOI TRE ONLINE".
- Bài nhiều tác giả (`THUẬN VĂN`, `TRÍ ĐỨC`) khớp byline nguồn.

**Chrome desktop:**
- Lọc theo nguồn Tuổi Trẻ Thế giới: icon tải được, nhãn "Toàn văn", giờ đăng khớp nguồn, chấm đỏ cho bài mới.
- Trang đọc bài: icon, tác giả, ảnh tải từ CDN (4/4 ở bài nhiều ảnh), caption/credit riêng, link "Đọc bài gốc tại Tuổi Trẻ…".
- Thẻ nguồn: 3 feed Tuổi Trẻ với icon và chuyên mục đúng. Ô chọn nguồn có "Tuổi Trẻ".
- 0 lỗi CSP/console. Mốc đã xem không đổi (chỉ lọc theo nguồn, không mở chuyên mục).

**Mobile 375px** (Chrome headless, CDP, profile tạm): tin, đọc bài Tuổi Trẻ, nguồn: không tràn ngang.

## 4. Dạng chưa hỗ trợ / giới hạn

- Kết luận dựa trên 9 trang khảo sát tay và 48 bài tự động trong 3 chuyên mục. **Không khẳng định hỗ trợ mọi dạng bài Tuổi Trẻ.**
- Chưa gặp và chưa xử lý riêng: emagazine/longform, infographic, album ảnh (`LayoutAlbum`), bài tường thuật trực tiếp, bài có bảng. Hộp có `type` lạ bị bỏ qua, nên có thể mất ảnh hoặc chữ. Nếu phần còn lại quá ít chữ, bài giữ tóm tắt.
- Trang `/video/` và bài chủ yếu là video chỉ có tóm tắt. Video nhúng trong bài thường bị bỏ.
- Tác giả viết hoa đúng như nguồn (ví dụ "THÁI BÁ DŨNG"); học hàm như "TS" giữ nguyên. Bài không có byline thì không có tác giả.
- Màn hình có mật độ điểm ảnh cao có thể tải file ảnh gốc (đến khoảng 2.500px, có khi là PNG) qua `srcset`, vì không có bản trung gian đã xác minh.
- Feed không hỗ trợ ETag nên mỗi 2 phút đọc lại 3 feed đầy đủ.
- Có 6 bài VnExpress/BBC cũ chứa ký tự Unicode dạng tách (NFD). Lượt này chỉ chuẩn hoá Tuổi Trẻ, không đụng dữ liệu nguồn khác.
- Quyền sử dụng toàn văn của Tuổi Trẻ chưa được xem xét (giống các nguồn khác, xem AUDIT_REPORT.md).

## 5. Kiểm tra

| Nhóm | Kết quả |
|---|---|
| gofmt, go vet, `go mod verify`, govulncheck | sạch / qua / qua / không có lỗ hổng (`golang.org/x/text` vốn đã có trong go.sum, nay là dependency trực tiếp; không thêm module mới) |
| Go `internal/news` | 33 qua. Thêm: trích xuất Tuổi Trẻ (thứ tự, caption/credit, bỏ liên quan/video/"Đọc tiếp"/bình luận/byline dưới, host ảnh lạ, hộp `content`, dấu câu); tác giả (nhiều người, bỏ MEDIA/TUỔI TRẺ ONLINE, không byline); không gắn toàn văn cho video/teaser/cấu trúc lạ, không panic; allowlist feed/bài (`/nld/`, subdomain, http, adapter khác); mã bài (slug khác cùng mã, `/video/`, query, path lồng); giờ đăng U+202F, AM/PM, chặn tương lai, không áp cho adapter khác; ảnh RSS; NFD → NFC |
| Go `cmd/server` (unit + integration PostgreSQL 17 tạm, đã xoá) | 25 qua. Thêm: migration thêm đúng 3 feed và không nhân bản khi chạy lại; bài ở 2 feed + đổi slug → 1 bản ghi, 2 chuyên mục, giờ UTC+7 đúng. CSP test cập nhật host mới |
| Node | 12 qua |
| Docker build (gồm vet + test) | qua; app healthy, 0 restart, 0 panic, 0 lần 429 trong log |
| Truy cập local | không mã → 200; Host lạ, `X-Forwarded-For`, POST `text/plain` → 401; thêm feed `nld.tuoitre.vn` → 400 |

---

# Lượt 6 — xem video (2026-10-05)

## 1. Khảo sát (bài thật, từ máy này)

| Nguồn | Bài mẫu | Trang bài / media / embed | Kiểm tra truy cập | Kết luận |
|---|---|---|---|---|
| Tuổi Trẻ | https://tuoitre.vn/video/bat-trom-2700-con-meo-o-tphcm-12-nguoi-bi-khoi-to-203231.htm (và trang `/video/…-203229.htm`) | Trang `/video/…`. Media: JSON-LD `VideoObject.contentUrl` = `https://cdn2.tuoitre.vn/…mp4`. `data-src` của `VideoStream` là URL player của nhà cung cấp (có `_info=<khoá>`), không dùng. | 206 khi có Range; `Access-Control-Allow-Origin: *`; không token/cookie; không phụ thuộc Referer/Origin (kiểm lại lúc 12:13 UTC); `moov` ở đầu file | **MP4 phát trực tiếp** |
| VnExpress | https://vnexpress.net/chay-xuong-nhua-phu-tung-xe-may-o-ngoai-thanh-ha-noi-5128489.html (và `…flydubai…-5128133.html`) | Video nhúng trong bài: `box_embed_video_parent` + `<video src="https://d1.vnecdn.net/<mục>/video/video/web/mp4/…/vne/master.m3u8">`, segment mã hoá AES-128. Không tìm thấy endpoint embed chính thức. | Lúc đầu: playlist, key và segment đều 200, CORS `*`. **Kiểm lại lúc 12:09 UTC: 407/406/405 "forbidden" trừ khi Referer là `vnexpress.net`** (chặn hotlink). Trên Chrome thật, một video phát được, video khác đứng ở 0 giây dù đã tải segment. | **Fallback**: poster + caption + "Xem video tại nguồn". Không giả Referer, không vượt chặn. |
| BBC | https://www.bbc.co.uk/news/articles/cme3r9ny2p4xo, https://www.bbc.co.uk/news/videos/c3eweld0nddeo | Dữ liệu trong `window.__INITIAL_DATA__`: khối `media` (vpid, poster `holdingImageUrl`, `isEmbeddingAllowed`, `externalEmbedUrl` = `/ws/av-embeds/articles/<id>/<vpid>/en-GB/`). Media qua mediaselector của player SMP. | Trang `/news/videos/`: 13/13 video `isEmbeddingAllowed:false`. Bài có video được phép nhúng: trang embed trả 200, không có `frame-ancestors`/XFO, nhưng khi nhúng trong iframe từ origin local thì **chỉ hiện khung trắng** (mở trực tiếp thì hiển thị). Không dùng mediaselector (API nội bộ của player, có quảng cáo/giới hạn vùng). | **Fallback**: poster + caption + "Xem video tại nguồn" |

Đã kiểm tra: link có hết hạn hay không, cookie/token, hotlink, CORS, giới hạn vùng (BBC đổi sang `bbc.com` ngoài Anh). Không vượt paywall/DRM/CAPTCHA/đăng nhập, không dùng dịch vụ tải video. **Phát được về kỹ thuật không có nghĩa là được phép tái sử dụng**; quyền sử dụng video chưa được xem xét.

## 2. Đã làm

- **Khối `video`** trong `blocks`, theo thứ tự trong bài:
  - Trường: `kind` (`mp4` | `link`), `src` (chỉ với `mp4`), `poster`, `width`/`height`, `duration`, `title`, `caption`, `credit`, `page_url`.
  - Không lưu HTML/script player, cookie hay token.
  - Poster chỉ nằm trong khối video, không bao giờ thành ảnh nội dung.
  - URL media chỉ nhận HTTPS, host/path đã xác minh (`cdn2.tuoitre.vn/…mp4`, không query, không phải `/1.1/` player). URL khác → `link`.
- **Extractor:**
  - VnExpress: khối video nhúng → `link` (poster `thumb-above-video` hoặc `img` trong `box_img_video`, caption "… Video: X" → credit).
  - BBC: `data-block="media"` khớp theo thứ tự với khối `media` trong `__INITIAL_DATA__`; chỉ lấy video (`programme`), bỏ audio. Trang `/news/videos/` lấy video chính.
  - Tuổi Trẻ: trang `/video/` → MP4 từ `VideoObject`. Chấp nhận JSON-LD có xuống dòng thô trong chuỗi (gặp trên trang thật). `VideoStream` trong bài → `link`.
- **Trang chủ yếu là video:** giữ tóm tắt RSS, không gắn "Toàn văn" (`content_status` vẫn `unavailable`), chỉ lưu khối video, không thử lại vô ích (`attempts=3`).
- **Nhãn "Video":** cột `has_video` (migration 008) chỉ phản ánh loại nội dung; trạng thái "Toàn văn"/"Chỉ có tóm tắt" vẫn hiện riêng.
- **Backfill có giới hạn:**
  - `ExtractVersion` 4: mỗi bài đã lưu được đọc lại **một lần**, tối đa 6 bài/site/lượt sau tin mới, giữ khoảng nghỉ và xử lý 429.
  - Bài chỉ có tóm tắt (đã hết lượt thử) cũng được đọc lại một lần để bổ sung video; tóm tắt/trạng thái không bao giờ bị đổi.
  - Lỗi mạng hoặc lỗi trích xuất không ghi đè nội dung tốt.
  - Migration 009/010 chỉ đánh dấu đọc lại các bài VnExpress lưu sai ở bản đầu (đường dẫn theo chuyên mục, thiếu poster). Migration 011 chuyển các khối HLS đã lưu sang `link`.
- **Feed `https://tuoitre.vn/video.rss`** (Khác) thêm theo xác nhận của bạn: nằm trong danh sách chính thức, GET 200, 50 item.
- **Player** (`web/video.js`, không thêm thư viện):
  - Chỉ tạo `<video controls preload="metadata">` khi người dùng bấm "Xem video"; không autoplay, không tải trước toàn bộ.
  - Trạng thái "Đang tải video…", không hỗ trợ định dạng, lỗi nguồn, quá 20 giây.
  - Không tự retry, không tự mở tab. Luôn có "Xem video tại nguồn" và link bài gốc.
  - Chuyển bài / quay lại danh sách: dừng, gỡ `src` + `load()` để huỷ request, xoá player, bỏ qua sự kiện muộn.
- **CSP:** thêm đúng `media-src 'self' https://cdn2.tuoitre.vn`. Không có `frame-src`, `worker-src`, `blob:`, wildcard hay `unsafe-eval`. Không có proxy hay endpoint tải video.

## 3. Kết quả

- Lúc 12:14 UTC (backfill đang chạy, done 603 / pending 372, 0 lỗi):
  - Tuổi Trẻ 61 bài có video (58 chỉ video), BBC 51 (17 trang video), VnExpress 36.
  - Khối video: Tuổi Trẻ 57 MP4 + 10 link; BBC 62 link; VnExpress 60 link.
- **Chrome thật (desktop, cửa sổ hiển thị):** MP4 Tuổi Trẻ trong app, bấm nút thật:
  - Thời gian tăng (16,8 → 18,8 giây), pause giữ vị trí, tua tới 40 giây, fullscreen vào được (`fullscreenElement=VIDEO`; thoát bằng API vì phím Escape giả lập không được xử lý).
  - Chuyển bài và quay lại danh sách dừng video, gỡ `src`, xoá player.
- **Fallback:**
  - Video có URL đúng host nhưng không tồn tại → "Không phát được video từ nguồn…", player bị gỡ, poster/nút/link giữ nguyên.
  - Media từ host lạ bị CSP chặn.
  - VnExpress/BBC hiển thị poster + "chỉ xem được tại …" + link, không tạo `<video>`.
- **Mobile 375px** (Chrome headless qua CDP, profile tạm, chạm thật):
  - Khung video 343×193, không tràn ngang.
  - MP4 Tuổi Trẻ: một lần phát 4,1 → 7,1 giây và tua được; lần chạy trước đó dừng ở 1,5 giây trong headless.
- **Chưa kiểm tra trên điện thoại thật.**
- Bài video không bị gắn "Toàn văn"; bài văn bản/ảnh cũ đọc bình thường; tìm kiếm, chấm đỏ và trạng thái đã xem không đổi.
- Trong lúc kiểm tra có lúc cửa sổ Chrome bị che (`document.hidden=true`), khi đó video không phát; kết quả trên chỉ tính lúc cửa sổ hiển thị.

## 4. Giới hạn

- Chỉ MP4 Tuổi Trẻ (trang `/video/`) phát trong app. Video nhúng trong bài Tuổi Trẻ chưa xác minh được URL media nên là link.
- VnExpress chặn hotlink tại thời điểm kiểm tra. Nếu sau này có cơ chế nhúng chính thức thì cần khảo sát lại.
- BBC: embed chính thức không hiển thị khi nhúng từ origin local. Có thể khác khi chạy trên domain thật; chưa kiểm tra.
- MP4 Tuổi Trẻ có thể rất nặng (một file khoảng 125 MB); trình duyệt tải dần theo Range.
- URL MP4 không có thời hạn tại thời điểm kiểm tra. Nếu nguồn đổi chính sách, player báo lỗi và chuyển sang fallback; không có cơ chế làm mới tự động vì hiện không cần.

---

# Lượt 7 — import nguồn bằng URL (2026-10-05)

## 1. Giao diện

- Ở đầu màn hình Nguồn tin: ô "URL nguồn tin" (placeholder "Dán URL website hoặc RSS…") và nút "Import nguồn".
- Trạng thái đang import / thành công / một phần / thất bại. Khi đang xử lý: nút bị khoá, ô chỉ đọc, submit lặp bị bỏ qua.
- Form thêm feed thủ công giữ lại (ghi là "nâng cao").
- Khi thất bại, một phần thất bại hoặc URL bị từ chối:
  - nút "Báo lỗi qua email" (`mailto:trinhthimai1509@gmail.com?subject=L%E1%BB%97i%20import&body=<URL đã trim, encodeURIComponent>`; chỉ mở khi bấm);
  - nút "Sao chép URL" và địa chỉ email để gửi thủ công.
- Không hiện nút này khi thành công, khi nguồn đã có, khi bị giới hạn tần suất (429) hoặc lỗi xác thực. Lỗi xác thực đưa về màn hình đăng nhập.

## 2. Nhận diện (chỉ từ URL, không gửi request; `news.ResolveImport`)

| Dạng URL | VnExpress | BBC | Tuổi Trẻ |
|---|---|---|---|
| Trang chủ / trang danh sách RSS | `vnexpress.net/`, `www.vnexpress.net`, `/rss` → 9 feed đã kiểm tra | `www.bbc.co.uk/`, `www.bbc.com/`, `bbc.co.uk`, `bbc.com` → 6 feed | `tuoitre.vn/`, `www.tuoitre.vn`, `/rss.htm` → 4 feed |
| RSS | `vnexpress.net/rss/<mục>.rss` | `feeds.bbci.co.uk/news/…/rss.xml` | `tuoitre.vn/<mục>.rss`, `/rss/<mục>.rss` (gộp về dạng đầu) |
| Chuyên mục | **không ánh xạ** (trang chuyên mục không khai báo RSS, trang RSS chỉ liệt kê feed) → hướng dẫn dán URL RSS | `/news`, `/news/world`, `/news/business`, `/news/technology`, `/news/entertainment_and_arts`, `/news/health` (theo `<channel><link>` của feed) | `/thoi-su.htm`, `/the-gioi.htm`, `/kinh-doanh.htm` (theo `<channel><link>`) |
| Bài viết | `….html` | `/articles/`, `/videos/`, `/live/`, `/av/` | `…-<mã>.htm`, `/video/…-<mã>.htm` → "Đây là URL của một bài viết…" |
| Khác | website khác → "Nguồn này chưa được hỗ trợ"; http, credential, cổng ≠ 443, URL lỗi → từ chối trước khi gửi request |

- Feed trong danh mục có tên và chuyên mục đã xác minh. RSS ngoài danh mục: tên lấy từ tiêu đề kênh (chữ thuần, ví dụ "Tuổi Trẻ Giáo dục"), chuyên mục Khác (sửa được trên thẻ nguồn).
- Các biến thể `www.` được chấp nhận vì đã kiểm tra chúng chuyển hướng về host chính.

## 3. Lưu và worker

- **Mỗi feed:**
  - Đã có và đang bật → "đã có", không đổi tên/chuyên mục.
  - Đang tắt → bật lại, báo "đã bật lại".
  - Đã xoá mềm → đọc thử RSS, khôi phục theo quy tắc hiện tại (bài cũ hiện lại), giữ tên/chuyên mục, báo "đã khôi phục".
  - Mới → đọc và parse RSS (cùng client của worker: allowlist, chỉ IP công khai, redirect trong allowlist, timeout, giới hạn 4 MB), rồi `INSERT … ON CONFLICT DO NOTHING`.
- **Kết quả:**
  - "Thành công" chỉ khi mọi feed đã đọc/parse được và đã lưu. Feed lỗi không được lưu.
  - Nhiều feed có lỗi → "Import chưa trọn vẹn", kèm số đã thêm / khôi phục / đã có / lỗi. Tất cả đã có → "Nguồn đã có sẵn và đang bật".
- **Giới hạn:** một lượt import tại một thời điểm, 6 lượt/phút, tối đa 20 feed/lượt, 90 giây/lượt, nghỉ 1 giây giữa các lần đọc feed.
- **Worker:** sau khi thêm/khôi phục/bật, server gửi một tín hiệu qua kênh 1 chỗ để worker chạy một lượt bình thường (advisory lock, nhịp theo site, 429). Không tạo goroutine crawl riêng; nhiều lần import không xếp hàng quá một lượt.
- API `POST /api/sources/import` nằm sau xác thực hiện có (mã truy cập hoặc chế độ local, gồm kiểm tra Origin/Content-Type).

## 4. Kiểm tra

- **Go:** `internal/news` có nhận diện (≈40 URL), danh mục, tiêu đề feed. `cmd/server` có:
  - import trang chủ giữ tên/chuyên mục tuỳ chỉnh, khôi phục, bật lại, chuyên mục theo danh mục, gửi tín hiệu worker; import lại không thêm gì;
  - một phần lỗi; RSS ngoài danh mục; URL bị từ chối không gửi request nào;
  - 4 request đồng thời không tạo trùng; 401 khi không có mã; giới hạn tần suất; đặt tên.
- **Node:** mailto (người nhận, tiêu đề, body bằng URL đã trim kể cả dấu cách/query/`#`/`+`/`%`/tiếng Việt; không có trường khác), và khi nào hiện nút báo lỗi.
- **Server thật (curl):**
  - `169.254.169.254`, `127.0.0.1:8080`, `localhost`, `[::1]`, `feeds.bbci.co.uk.evil.example`, `vnexpress.net@evil.example`, `file:` và `/nld/` đều bị từ chối, không gửi request.
  - Lượt thứ 6 trong một phút → 429. POST cross-site và `text/plain` → 401.
- **Chrome desktop với nguồn thật:**
  - Trang chủ cả 3 nguồn, RSS VnExpress, chuyên mục BBC và Tuổi Trẻ → "đã có", không có nút báo lỗi.
  - Chuyên mục VnExpress, URL bài, website khác, http, credential, cổng lạ → thông báo đúng, có nút báo lỗi; mailto đúng.
  - Khôi phục "BBC Science (tạm)": giữ tên, worker lấy tin ngay (37 bài).
  - Feed 404 thật → thất bại, không lưu.
  - Thêm `tuoitre.vn/giao-duc.rss`: thẻ hiện ngay, đang bật, icon đúng, chuyên mục Khác; worker lấy 50 bài ở lượt kế tiếp.
  - Hai feed thử đã được xoá mềm lại sau kiểm tra.
  - Ba lần submit chỉ tạo 1 request. 401 (giả lập) → màn hình đăng nhập, không có nút báo lỗi. Trạng thái đã xem không đổi.
- **Mobile 375px** (CDP headless): form và kết quả lỗi không tràn ngang, nút xếp dọc.
- **Chung:** govulncheck sạch; Docker build qua; app healthy, 0 restart, 0 panic.

## 5. Giới hạn

- URL chuyên mục VnExpress chưa tự chuyển sang RSS. Chuyên mục BBC/Tuổi Trẻ ngoài danh mục cũng vậy.
- Tên feed ngoài danh mục lấy từ tiêu đề kênh. Feed BBC có tiêu đề chung "BBC News", nên tên được ghép từ đường dẫn.
- Lượt import chỉ kiểm tra RSS, không chờ toàn văn; bài mới về ở lượt worker kế tiếp.
- Giới hạn tần suất là toàn cục (app một người dùng).

---

# Lượt 8 — một luồng thêm nguồn duy nhất (2026-10-05)

- **Một form thêm nguồn:** "URL nguồn tin" + nút "Thêm nguồn", gợi ý "Hỗ trợ VnExpress, BBC News và Tuổi Trẻ."
  - Logic nhận diện/kiểm tra RSS/lưu của lượt 7 giữ nguyên: nguồn đã có, khôi phục nguồn xoá mềm, lỗi một phần, giới hạn tần suất, nút "Báo lỗi qua email".
- **Đã xoá form "Thêm kênh RSS thủ công (nâng cao)"** (chọn adapter, nhập tên/URL/chuyên mục).
  - **Đã gỡ API `POST /api/sources`:** trước khi gỡ đã kiểm tra, chỉ form này và một test còn dùng nó (README/HANDOVER không nhắc). Giờ trả 405.
  - Test validate URL cũ được chuyển sang endpoint thêm nguồn (`/api/sources/import`).
  - RSS thuộc adapter hỗ trợ nhưng chưa có ánh xạ chuyên mục vẫn thêm được qua form mới: chuyên mục Khác, tên lấy từ tiêu đề kênh.
- **Thẻ nguồn:**
  - Thêm ô "Tên hiển thị" + "Lưu tên". `PATCH /api/sources/{id}` nhận thêm `name` (trim, 1–100 ký tự).
  - Chuyên mục, bật/tắt, xoá như cũ. Adapter và URL feed không đổi được: PATCH bỏ qua các trường đó; body chỉ có `adapter` → 400.
- **Thông báo rút gọn:**
  - "Đã thêm nguồn: <tên>." / "Đã thêm N kênh." / "Đã thêm N kênh, M kênh lỗi.", kèm "Tin mới sẽ được cập nhật tự động." khi có kênh mới.
  - "Nguồn này đã có trong danh sách."
  - "Không thêm được nguồn: <lý do>."
  - Danh sách từng kênh nằm trong phần "Chi tiết" (thu gọn).
- **Không đổi:** logic lấy tin, bảo mật, dữ liệu, trạng thái đã xem.

**Kiểm tra:**
- **Go:** 76 test (gồm integration PostgreSQL 17 tạm, đã xoá). Mới thêm: sửa tên/chuyên mục không đổi được adapter/URL; tên rỗng/quá dài bị từ chối; nội dung các thông báo; endpoint cũ trả 405.
- **Node:** 19 test. govulncheck sạch. Docker build qua.
- **Chrome desktop với nguồn thật:**
  - Form mới, không còn form thủ công.
  - RSS và trang chủ đã có → "Nguồn này đã có trong danh sách." (chi tiết thu gọn, không có nút email).
  - Chuyên mục VnExpress và website khác → lỗi rõ ràng, có nút email.
  - `tuoitre.vn/rss/giao-duc.rss` (đã xoá mềm) → khôi phục, giữ tên cũ. Đổi tên thành "Tuổi Trẻ Giáo dục" (đã trim) và chuyên mục sang Thời sự qua UI; URL feed và icon adapter không đổi; tên rỗng bị bỏ qua.
  - Feed thử đã được xoá mềm lại. Trạng thái đã xem không đổi.
- **Mobile 375px** (CDP headless): form, kết quả trùng/lỗi và thẻ nguồn có ô sửa tên không tràn ngang.

---

# Video demo (2026-10-05)

- `demo/tin-rieng-demo-desktop-1440x900.mp4`: 113,8 giây, H.264, 1440×900, 30 fps.
- `demo/tin-rieng-demo-mobile-gia-lap-390x844.mp4`: 113,4 giây, H.264, 780×1688 (viewport 390×844, tỷ lệ điểm ảnh 2), dọc. **Giả lập trình duyệt (Chrome headless, mobile + touch qua DevTools), không phải điện thoại thật.**
- **Cách quay:** app local thật, dữ liệu thật, Chrome headless với profile tạm; thao tác bằng sự kiện chuột/chạm thật; khung hình lấy bằng screencast; ghép và thêm phụ đề tiếng Việt bằng ffmpeg chạy trong container tạm (đã xoá).
  - Chấm đỏ tròn trên hình là con trỏ minh hoạ chèn tạm lúc quay, không phải giao diện app.
  - Để có chấm đỏ "bài mới", profile tạm được đặt mốc "đã xem" như người dùng mở app khoảng 2,5 giờ trước. Trạng thái đã xem của trình duyệt thật không bị đụng tới.
- **Nội dung:** bản tin 3 nguồn; chấm đỏ; lọc chuyên mục; tìm theo tiêu đề; đọc toàn văn có tác giả/ảnh/chú thích, nguồn và link gốc; phát video MP4 Tuổi Trẻ; fallback video BBC; quản lý nguồn; thêm nguồn bằng URL (nguồn đã có, nguồn chưa hỗ trợ kèm nút báo lỗi).
  - Không thêm hay sửa nguồn nào trong lúc quay.
- Thư mục `demo/` đã được loại khỏi image Docker và git.

---

# Lượt 9 — lọc theo quốc gia của nguồn báo; đọc công khai, quản trị một tài khoản (2026-10-06)

## 1. Quốc gia của nguồn báo

- **Định nghĩa:** quốc gia của tòa soạn/website đăng bài, không phải quốc gia được nhắc trong bài. Không dịch, không phân loại bằng AI, không suy từ tên miền/tiêu đề. Quốc gia và ngôn ngữ là hai thứ riêng (BBC tiếng Anh → Vương quốc Anh; nếu sau này có BBC Tiếng Việt thì vẫn là Vương quốc Anh).
- **Mô hình:** migration `012_countries.sql`:
  - `countries` (mã ISO 3166-1 alpha-2, tên tiếng Việt, thứ tự; 66 nước gồm Thái Lan, Trung Quốc). Thêm nước không tạo nguồn hay bài nào.
  - `publishers` (một dòng/website, khoá = adapter đang có: `vnexpress`, `bbc`, `tuoitre`), cột `country` NULL = "Chưa xác định". Gán sẵn: VnExpress, Tuổi Trẻ → VN; BBC → GB.
  - FK `sources.adapter → publishers.adapter`: mọi feed thuộc một website, nên mọi feed của cùng website luôn cùng quốc gia (không có quốc gia riêng từng feed).
  - Chỉ thêm, idempotent (`IF NOT EXISTS`, `ON CONFLICT DO NOTHING`, kiểm tra constraint trước khi thêm); chạy lại không ghi đè lựa chọn của quản trị viên.
- **API:** `GET /api/articles?country=VN|GB|…|unknown` (kết hợp `category`, `source`, `q`, `page`); bài và nguồn trả thêm `country`. `GET /api/countries` (công khai): danh sách nước, `in_use`, `unknown_in_use`. Quản trị: `GET /api/admin/publishers`, `PATCH /api/admin/publishers/{adapter}` với `{"country":"TH"}` hoặc `{"country":null}`. Sửa feed (`PATCH /api/admin/sources/{id}`) không nhận quốc gia.
- **Giao diện:** bộ lọc "Quốc gia nguồn" (chỉ nước đang có nguồn, thêm "Chưa xác định" nếu có website chưa gán); danh sách nguồn thu theo quốc gia, nguồn đang chọn bị bỏ nếu không thuộc quốc gia mới; đổi bộ lọc về trang 1. Màn hình đọc bài: "Nguồn: VnExpress · Việt Nam · …". Quản trị: khung "Quốc gia của website" (một ô chọn/website, ghi rõ áp dụng cho mọi feed), thẻ nguồn hiện "Quốc gia: … (theo website)".
- **Đã xem / chấm đỏ:** số bài mới của chuyên mục không phụ thuộc bộ lọc quốc gia. Mở chuyên mục khi đang lọc quốc gia (cũng như nguồn/từ khoá) **không** đánh dấu đã xem (`seen.js`, `markSeen` thêm điều kiện `country`). Chuyên mục, chấm đỏ giữ nguyên.

## 2. Đọc công khai, quản trị một tài khoản

- **Công khai (GET, không cần đăng nhập):** `/api/articles`, `/api/articles/{id}`, `/api/categories`, `/api/countries`, `/api/sources` (chỉ `id,name,adapter,country`), `/api/admin/session` (khách chỉ nhận `{"authenticated":false}`), `/healthz`. Phương thức khác trên các đường này → 404/405.
- **Quản trị (`/api/admin/*`, cần phiên):** `GET sources` (đủ chi tiết), `PATCH/DELETE sources/{id}`, `POST sources/import`, `GET status`, `GET publishers`, `PATCH publishers/{adapter}`, `POST logout`. Đường lạ dưới `/api/admin/` trả 401 cho khách. Các đường cũ (`PATCH/DELETE /api/sources/{id}`, `POST /api/sources/import`, `GET /api/status`, `GET /api/session`) đã gỡ.
- **Gỡ bỏ:** Bearer `ADMIN_TOKEN` (cùng giới hạn sai mã cũ), chế độ `LOCAL_NO_AUTH` (`local.go`, `PUBLISHED_HOST`), token trong sessionStorage. Nếu `.env` còn hai biến này, app bỏ qua và ghi log nhắc; không còn đường ghi nào không qua phiên quản trị.
- **Phiên** (`cmd/server/auth.go`, migration `013_admin_auth.sql`):
  - Mật khẩu: argon2id (m=64 MiB, t=3, p=2, salt 16 byte), PHC string; tối đa 2 phép băm song song; tên sai vẫn băm với hash giả (thời gian trả lời như nhau).
  - Đăng nhập tạo token ngẫu nhiên 256 bit mới (xoá phiên cũ của trình duyệt đó). DB chỉ lưu SHA-256 của token + CSRF token. Cookie `nr_admin` (`__Host-nr_admin` khi `COOKIE_SECURE=true`): HttpOnly, SameSite=Strict, Path=/, Max-Age = TTL, Secure theo cấu hình.
  - Hết hạn: tuyệt đối `ADMIN_SESSION_TTL` (12h), không hoạt động `ADMIN_SESSION_IDLE` (2h); dọn phiên hết hạn mỗi giờ và khi đăng nhập. Đăng xuất xoá dòng phiên. `reset-password` xoá mọi phiên.
  - CSRF cho mọi request ghi: header `X-CSRF-Token` khớp phiên (so sánh hằng thời gian), `Origin` (nếu có) trùng Host, `Sec-Fetch-Site` (nếu có) là `same-origin`, POST/PATCH phải `application/json`. Đăng nhập cũng kiểm tra Origin/Sec-Fetch-Site/JSON.
  - Giới hạn đăng nhập: 5 lần sai/15 phút/địa chỉ, 30 lần sai/15 phút tổng → 429 + Retry-After; kiểm tra trước khi đọc DB hay băm. Lỗi chung "Tên đăng nhập hoặc mật khẩu không đúng".
  - Log chỉ ghi "admin login from/failed from <địa chỉ>"; không ghi tên đã nhập, mật khẩu, cookie, token.
- **CLI** (`server admin …`, `cmd/server/admin_cli.go`): `create <tên>`, `reset-password`, `logout-all`, `status`. Mật khẩu hỏi 2 lần không hiện ký tự (x/term) hoặc dòng đầu stdin; 12–128 ký tự; một tài khoản (unique index `admin_users_single`). Không có tài khoản/mật khẩu mặc định.
- **Giao diện:** mở trang là đọc ngay. Header: "Đọc tin", "Đăng nhập quản trị" (khách) / "Nguồn tin", "Đăng xuất" (quản trị). Form thêm nguồn và thẻ nguồn chỉ hiện sau khi đăng nhập (và backend vẫn từ chối nếu gọi trực tiếp). Phiên hết hạn giữa chừng → về màn hình đăng nhập với thông báo. CSRF token chỉ giữ trong bộ nhớ trang.
- **Báo lỗi qua email:** giữ nguyên hành vi (mailto chỉ chứa URL đã nhập). Địa chỉ nhận không còn nằm trong `importer.js` công khai; lấy từ `REPORT_EMAIL` (mặc định như cũ) và chỉ trả cho quản trị viên trong `/api/admin/session`.
- **Cấu hình mới:** `COOKIE_SECURE` (false/true), `ADMIN_SESSION_TTL`, `ADMIN_SESSION_IDLE`, `REPORT_EMAIL` (compose.yaml, .env.example). Dependency mới: `golang.org/x/crypto` (argon2), `golang.org/x/term`.

## 3. Lỗi phát hiện khi kiểm tra, đã sửa

- `PATCH /api/admin/publishers/{adapter}` với `{"country":null}` bị trả 400 (JSON `null` giải mã thành con trỏ nil, không phân biệt được với thiếu khoá) → đổi sang map để phân biệt; có test.
- Mobile 375px: ô chọn quốc gia trong khung "Quốc gia của website" tràn 11px → thêm `min-width:0`/`max-width:100%`; đo lại không tràn.
- Test thời hạn phiên 2 s chập chờn do đồng hồ VM Colima lệch ~0,1 s giữa container → tăng biên (TTL 1 s, chờ 2,5 s); không phải lỗi app.

## 4. Kiểm tra

Chi tiết số liệu: VALIDATION.md (Lượt 9). Tóm tắt: migration giữ nguyên dữ liệu (1371 bài/1233 toàn văn/23 nguồn) và chạy lại an toàn; Go 82/82 (PostgreSQL 17 tạm), Node 23/23, vet/gofmt sạch, govulncheck không có lỗ hổng có đường gọi; mọi API quản trị từ chối khách, Bearer cũ, header giả local và cookie giả; quản trị viên đăng nhập, import, sửa quốc gia, bật/tắt, xoá trên stack thử (bản sao dữ liệu); đăng xuất/hết hạn/idle/giới hạn đăng nhập hoạt động; Chrome desktop thật; mobile 375px chỉ bằng giả lập.

Test mới: `auth_test.go` (hash, khách bị từ chối mọi route quản trị, API công khai chỉ đọc, CSRF đăng nhập + giới hạn, thuộc tính cookie, luồng đăng nhập/CSRF/đăng xuất, hết hạn tuyệt đối/idle/TTL ngắn, giới hạn với DB, CLI, API công khai không lộ dữ liệu quản trị), `countries_test.go` (lọc quốc gia kết hợp + phân trang + chấm đỏ không đổi, quản trị quốc gia website, chạy lại migration), `tests/filters.test.js`, bổ sung `seen.test.js`, `importer.test.js`.

## 5. Giới hạn còn lại

- Một quản trị viên; không có đăng ký, phân vai, 2FA, khôi phục mật khẩu qua email. Giới hạn đăng nhập nằm trong bộ nhớ (mất khi khởi động lại; nhiều instance thì mỗi instance đếm riêng).
- Sau reverse proxy mọi request cùng địa chỉ nguồn → giới hạn theo địa chỉ thành giới hạn chung (khoá cả quản trị viên tối đa 15 phút nếu bị dò); nên rate limit ở proxy. Chưa triển khai/kiểm tra qua HTTPS thật (`COOKIE_SECURE=true` chỉ được kiểm bằng unit test thuộc tính cookie).
- Quốc gia chỉ gán được cho 3 website có adapter; chưa có nguồn Thái Lan/Trung Quốc (không thêm crawler mới trong lượt này).
- Mobile 375px chỉ kiểm bằng giả lập; chưa thử trên điện thoại thật.
- `.env` của bạn vẫn còn dòng `ADMIN_TOKEN`, `LOCAL_NO_AUTH` (tôi không sửa file bí mật của bạn); chúng không còn tác dụng, có thể xoá.
- Chưa có tài khoản quản trị trên app local: cần chạy `docker compose exec app /app/server admin create <tên>`.
