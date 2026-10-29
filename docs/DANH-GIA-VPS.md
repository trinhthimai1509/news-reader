# Đánh giá nhu cầu RAM/CPU để chọn VPS — News Reader

Ngày đo: 2026-10-07. Mọi số trong tài liệu là **số đo thật trên máy phát triển** (macOS, Docker trong VM Colima) với giới hạn RAM mô phỏng, **không phải** kết quả trên VPS Linux thật. Chỗ nào là ước tính hoặc phân tích từ code đều ghi rõ.

## 0. Tóm tắt

| Câu hỏi | Trả lời ngắn | Căn cứ |
|---|---|---|
| Bản hiện tại chạy được trong ngân sách VPS 1 GB không? | **Có, dư nhiều** (với dữ liệu thật hiện có) | App cao nhất 14 MiB / 192 MiB, PostgreSQL cao nhất 77 MiB / 384 MiB; 0 OOM, 0 restart, 0 lỗi ở mọi mức thử, kể cả 50 phiên + worker + sao lưu |
| 2 GB có nhanh hơn không? | **Không**, với dữ liệu hiện tại và cả 302 nghìn bài | Kết quả 1 GB và 2 GB gần như trùng nhau; nút thắt khi dữ liệu lớn là **CPU của PostgreSQL**, không phải RAM |
| Điều kiện để dùng 1 GB | Không build image trên VPS; bật swap nhỏ làm đệm; giới hạn log Docker; reverse proxy nhẹ | Build trên máy đo dùng tới ~439 MiB RAM |
| Mục tiêu 200 feed | RAM vẫn đủ ở 1 GB (đo với 302 nghìn bài, 2,1 GB dữ liệu); **tìm kiếm và một số truy vấn chậm dần theo số bài**; ổ đĩa cần 50 GB | Mục 6, 7 |
| 200 website | **Chưa kiểm chứng được** (app mới có 3 adapter) | Mục 6.3 |
| Phải kiểm lại trên VPS thật | CPU (VPS chậm hơn Apple M5), RAM hệ điều hành thật, image amd64 | Mục 9 |

## 1. Bản được kiểm tra và môi trường

- Code: commit `dcb4991` **cộng thay đổi chưa commit** của lượt 10–11 (lượt đọc, chuyên mục v2, dropdown; migration tới `016`). Image `news-reader-app:latest` build lúc 18:01 ngày 2026-10-07 từ đúng thư mục này (kiến trúc **arm64**).
- Máy: MacBook Apple M5, 10 nhân, 16 GB. Docker 29.5 chạy trong VM Colima 0.10.3: Ubuntu 24.04, kernel 6.8, **2 vCPU, 2 GiB RAM, không swap**, cgroup v2.
- Trong VM lúc đo còn chạy: app thật (`news-reader-app-1`), database thật (`news-reader-db-1`) và PostgreSQL của dự án khác (`tu-van-phap-luat-postgres-1`). Không dừng, không cấu hình lại, không tạo tải lên các container này.
- PostgreSQL: `postgres:17-alpine`, cấu hình mặc định như `compose.yaml` (shared_buffers 128 MB, work_mem 4 MB, max_connections 100). App: pool tối đa 10 kết nối DB (`cmd/server/main.go`).

## 2. Thành phần sẽ chạy trên VPS và ngân sách RAM

| Thành phần | Có đo không | Số liệu |
|---|---|---|
| App Go (web + worker trong cùng tiến trình) | Đo | 4 MiB lúc rảnh, 9–14 MiB khi có tải/worker |
| PostgreSQL 17 | Đo | 30 MiB lúc rảnh; 40–77 MiB với dữ liệu hiện tại; 180–238 MiB với 302 nghìn bài |
| Docker (dockerd + containerd) | Đo trong VM Colima | ≈ 119 MiB RSS (70 + 49) |
| Hệ điều hành | **Không đo được trên VPS** | VM Colima (Ubuntu 24.04 tối giản + agent của Colima + phiên ssh): toàn VM dùng ≈ 539 MiB gồm cả Docker và 3 container (≈ 146 MiB). VPS Ubuntu tối giản thường 150–300 MiB — **ước tính**, phải đo lại |
| Reverse proxy HTTPS (Caddy/Nginx) | **Không đo** (repo chưa có cấu hình proxy) | Dành sẵn 64 MiB (1 GB) / 128 MiB (2 GB) — ước tính |

Giới hạn container đặt khi đo (`--memory` = `--memory-swap`, tức **không cho dùng swap**, vượt là OOM):

| Ngân sách | PostgreSQL | App | Còn lại cho hệ điều hành, Docker, proxy |
|---|---|---|---|
| VPS 1 GB | 384 MiB | 192 MiB | ≈ 448 MiB |
| VPS 2 GB | 768 MiB | 384 MiB | ≈ 896 MiB |

Giới hạn container trên Mac **chỉ là mô phỏng**: kernel của VM Colima giới hạn đúng như trên VPS (cgroup v2), nhưng CPU, đĩa và hệ điều hành khác VPS.

## 3. Phương pháp đo

- **Stack thử riêng:** mạng `nr-vps`, volume `nr-vps-pgdata`, container `nr-vps-db` và `nr-vps-app`, cổng 8083; app chạy như compose (read-only, bỏ mọi capability, `--init`). Dữ liệu: bản `pg_dump` của database thật lúc 19:28 (**1939 bài, 2192 liên kết bài–feed, 22 MB**); sau một lượt worker thử: 1971 bài, 21 MB.
- **Không crawl thêm nguồn thật:** mọi feed trong bản sao bị tắt. Hai lượt worker thật (mỗi ngân sách) chỉ bật 2 feed mà app thật **không** lấy (Tuổi Trẻ Giáo dục, BBC Science — đã xoá mềm ở app thật), xoá ETag và đưa 12 bài/feed về "chờ toàn văn", để worker làm việc thật trong giới hạn sẵn có (12 trang/site/lượt, cách 1,5 s). Không đổi cơ chế chống SSRF.
- **Đo RAM/CPU từ kernel** (`scripts/vps-test/sample.sh`): mỗi giây đọc cgroup của từng container trong VM: `memory.current` (mọi bộ nhớ tính cho container, gồm page cache và shared buffers của PostgreSQL), `anon` trong `memory.stat` (bộ nhớ tiến trình thật), `file` (page cache + shared buffers, phần lớn thu hồi được), `cpu.stat`, `memory.events oom_kill`; và `MemAvailable` của cả VM. Ba số này **không cộng lẫn nhau**: `cur = anon + file + kernel`. Đỉnh tổng của cả phiên đo lấy thêm từ `memory.peak`.
- **Số kết nối DB:** một tiến trình `psql` chạy liên tục `\watch 5` (không tính chính nó).
- **Tạo tải** (`scripts/vps-test/load.mjs`, Node trên macOS, **ngoài VM và ngoài ngân sách**): mỗi phiên mở trang (HTML, CSS, 9 file JS, các API khởi động), rồi cứ 2–5 s làm một việc: danh sách, lọc chuyên mục/quốc gia/nguồn, tìm kiếm, sang trang, mở bài (+ sự kiện "lượt đọc"); mỗi 60 s gửi heartbeat và đọc `/api/stats`. Không tải ảnh, không gọi báo/CDN. Độ trễ đo từ macOS; chi phí chuyển cổng Colima đã đo: `/healthz` p50 0,7 ms (từ Mac) so với 0,2 ms (trong VM).
- **Lưu ý về mã 429:** mọi phiên giả lập đi từ **một địa chỉ IP**, nên giới hạn 60 lượt đọc/phút/IP và 120 sự kiện thống kê/phút/IP của app chặn bớt sự kiện "lượt đọc"/heartbeat (429, đúng thiết kế). Cột "429 thống kê" ghi riêng, không tính là lỗi. Người dùng thật có IP khác nhau.
- Mỗi mức tải chạy 120 s, nghỉ 15 s giữa các mức; khởi động lạnh (dừng cả hai container rồi chạy lại) trước mỗi bộ đo.

## 4. Kết quả với dữ liệu thật (≈ 1970 bài, 21 MB)

RAM tính bằng MiB (giá trị cao nhất trong tình huống). CPU tính bằng % một nhân (200% = cả 2 vCPU), dạng trung bình/cao nhất. Độ trễ tính bằng ms cho mọi request của tình huống.

### 4.1. Ngân sách 1 GB (DB 384 MiB, app 192 MiB)

| Tình huống | Thời lượng | Request | req/s | Lỗi | 429 thống kê | p50 | p95 | Max | App RAM (anon) | DB RAM cur (anon / file) | CPU app | CPU DB | Kết nối DB | OOM |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Khởi động lạnh | DB sẵn sàng 0,44 s; app phục vụ 0,29 s sau đó; danh sách đầu tiên 13 ms | | | | | | | | | | | | | |
| Rảnh, worker không có feed | 60 s | – | – | – | – | – | – | – | 4,0 (3,4) | 29,9 (10,1 / 17,5) | 0,1/2,2 | 0,5/4,0 | 2 | 0 |
| Ít tải (1 phiên) + 1 lượt worker thật | 152 s | 83 | 0,5 | 0 | 0 | 13,2 | 36,5 | 73 | 11,7 (10,7) | 37,9 (15,8 / 19,5) | 1,4/13,4 | 0,9/6,5 | 3 | 0 |
| 5 phiên | 123 s | 351 | 2,8 | 0 | 0 | 7,1 | 22,1 | 41 | 9,1 (8,0) | 39,1 (16,6 / 19,8) | 0,7/4,8 | 1,4/5,8 | 3 | 0 |
| 10 phiên | 125 s | 695 | 5,6 | 0 | 0 | 6,2 | 16,3 | 29 | 9,2 (8,2) | 39,6 (16,7 / 20,3) | 1,0/4,1 | 1,9/6,4 | 3 | 0 |
| 20 phiên | 124 s | 1370 | 11,0 | 0 | 84 | 5,2 | 14,8 | 38 | 9,3 (8,3) | 40,0 (16,7 / 20,4) | 1,7/4,5 | 3,1/6,6 | 3 | 0 |
| 50 phiên | 124 s | 3364 | 27,1 | 0 | 328 | 3,9 | 13,1 | 66 | 9,5 (8,4) | 43,4 (19,8 / 20,5) | 2,8/5,5 | 6,2/13,4 | 4 | 0 |
| 20 phiên + lượt worker + sao lưu `pg_dump` | 155 s | 1672 | 10,8 | 0 | 75 | 5,6 | 15,5 | 35 | 12,7 (11,7) | 60,1 (23,2 / 33,1) | 2,5/12,8 | 3,8/54,8 | 5 | 0 |
| Ép tải: 20 phiên **không nghỉ** | 60 s | 156 420 | 2607 | 0 | 30 295 | 5,2 | 17,1 | 87 | 14,1 (12,6) | 77,2 (39,0 / 33,2) | 28,8/31,6 | 130,9/140,7 | 10 | 0 |

### 4.2. Ngân sách 2 GB (DB 768 MiB, app 384 MiB)

| Tình huống | Thời lượng | Request | req/s | Lỗi | 429 thống kê | p50 | p95 | Max | App RAM (anon) | DB RAM cur (anon / file) | CPU app | CPU DB | Kết nối DB | OOM |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Khởi động lạnh | DB 0,43 s; app 0,18 s sau đó | | | | | | | | | | | | | |
| Rảnh | 59 s | – | – | – | – | – | – | – | 3,9 (3,3) | 30,7 (11,0 / 17,2) | 0,1/2,1 | 0,5/6,9 | 3 | 0 |
| Ít tải + 1 lượt worker thật | 154 s | 80 | 0,5 | 0 | 0 | 16,8 | 36,5 | 53 | 11,4 (10,4) | 37,7 (15,9 / 19,2) | 1,1/9,2 | 0,9/9,8 | 3 | 0 |
| 5 phiên | 124 s | 338 | 2,7 | 0 | 0 | 6,6 | 17,8 | 36 | 9,1 (8,1) | 38,9 (16,6 / 19,7) | 0,6/3,4 | 1,2/5,4 | 3 | 0 |
| 10 phiên | 124 s | 687 | 5,6 | 0 | 0 | 6,4 | 17,0 | 40 | 9,5 (8,5) | 42,6 (19,7 / 20,1) | 1,0/4,1 | 1,9/8,4 | 4 | 0 |
| 20 phiên | 124 s | 1365 | 11,0 | 0 | 68 | 4,8 | 15,6 | 41 | 9,4 (8,4) | 43,1 (19,9 / 20,3) | 1,8/5,9 | 3,3/8,7 | 4 | 0 |
| 50 phiên | 124 s | 3450 | 27,8 | 0 | 352 | 3,9 | 13,6 | 39 | 10,6 (9,4) | 43,4 (19,9 / 20,4) | 2,9/6,9 | 6,2/12,9 | 4 | 0 |
| 20 phiên + worker + sao lưu | 154 s | 1660 | 10,8 | 0 | 66 | 5,4 | 15,1 | 36 | 13,4 (12,3) | 60,9 (25,7 / 33,3) | 2,0/9,1 | 3,7/26,0 | 5 | 0 |
| Ép tải: 20 phiên không nghỉ | 60 s | 152 359 | 2539 | 0 | 29 228 | 5,3 | 17,5 | 126 | 13,7 (12,5) | 76,9 (39,3 / 33,3) | 27,4/30,2 | 129,5/140,2 | 10 | 0 |

### 4.3. Khởi động lại, sao lưu

- `docker restart` app: phục vụ lại sau 0,29 s (cả hai ngân sách). Restart PostgreSQL khi app đang chạy: app tự kết nối lại, `/api/articles` trả 200 sau 0,60 s. Số bài (1971) và tổng lượt đọc trước/sau restart giống hệt.
- `pg_dump -Fc` khi đang có 20 phiên đọc + lượt worker: 0,5 s, file 4,1 MB; độ trễ người đọc không tăng (p95 15,5 so với 14,8 ms). CPU DB có đỉnh 55% trong lúc dump.
- Trong suốt các lượt đo: **0 OOM** (`memory.events oom_kill`), **0 restart**, **0 lỗi** HTTP ngoài 429 thống kê (giải thích ở mục 3). `memory.peak` (đỉnh cả phiên 1 GB): app 12,8 MiB / 192 MiB; DB 221 MiB / 384 MiB (đỉnh DB này gồm cả lúc nạp dữ liệu 302 nghìn bài ở mục 6).

## 5. Đọc kết quả

- **RAM không phải giới hạn ở quy mô hiện tại.** App dùng ~10 MiB; PostgreSQL ~40–77 MiB, trong đó phần lớn là page cache/shared buffers (cột "file"), không phải bộ nhớ tiến trình. Mức dùng gần như không đổi từ 5 đến 50 phiên.
- **Không có khác biệt 1 GB / 2 GB** ở mọi tình huống: chênh lệch nằm trong dao động đo (ví dụ p95 50 phiên: 13,1 so với 13,6 ms).
- **CPU:** 50 phiên đọc có nghỉ chỉ dùng ~6% một nhân cho DB. Khi ép tải không nghỉ, PostgreSQL dùng ~130% (gần hết 2 vCPU) — đây là trần CPU, RAM vẫn thấp. **CPU của Apple M5 nhanh hơn nhiều so với vCPU chia sẻ của VPS giá rẻ**, nên số req/s tối đa (≈ 2500) **không dùng được** cho VPS; chỉ biết là còn cách rất xa mức 28 req/s của 50 phiên.
- "50 phiên đồng thời" là mức thử, không phải cam kết số người dùng.

## 6. Mục tiêu 200 feed

### 6.1. Dữ liệu mô phỏng (chỉ trong stack thử)

`scripts/vps-test/seed-200.sql`: thêm **200 feed** (tắt, không crawl) thuộc 3 website có adapter: VnExpress 90, BBC 60, Tuổi Trẻ 50; chuyên mục lần lượt. Thêm **300 000 bài** chép nội dung của các bài toàn văn thật (nên kích thước giống bài thật), URL giả, ngày đăng rải đều 65 ngày (≈ 200 feed × 23 bài/ngày đo được ở app thật); mỗi bài thuộc 1 feed, 15% thuộc thêm feed thứ hai cùng website (bài trùng giữa feed). Kết quả: **301 971 bài, 223 feed, 347 238 liên kết, database 2 139 MB** (≈ 7,1 KB/bài).

### 6.2. Kết quả với 302 nghìn bài

| Ngân sách | Tình huống | Request | req/s | Lỗi | p50 | p95 | Max | App RAM | DB RAM cur (anon / file) | CPU DB tb/max | Kết nối DB | OOM |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 GB | 20 phiên | 1368 | 11,1 | 0 | 5,3 | **132** | 464 | 8,4 | 198,9 (33,0 / 160,3) | 23/95 | 4 | 0 |
| 1 GB | 50 phiên | 3458 | 27,7 | 0 | 2,9 | 38 | 273 | 10,7 | 203,2 (42,9 / 150,9) | 34/107 | 10 | 0 |
| 1 GB | Ép tải không nghỉ | 9042 | **149** | 0 | 52 | **562** | 1771 | 11,0 | 238,1 (64,0 / 164,2) | **178/192** | 10 | 0 |
| 2 GB | 20 phiên | 1368 | 11,0 | 0 | 5,2 | **113** | 345 | 8,3 | 177,6 (24,1 / 147,9) | 24/71 | 4 | 0 |
| 2 GB | 50 phiên | 3490 | 28,0 | 0 | 3,3 | 91 | 1710 | 10,8 | 199,9 (42,3 / 149,6) | 38/172 | 10 | 0 |
| 2 GB | Ép tải không nghỉ | 9040 | **150** | 0 | 52 | **567** | 1778 | 11,2 | 214,6 (53,8 / 150,9) | **184/190** | 10 | 0 |

Theo loại request (1 GB, 20 phiên): **tìm kiếm** p50 144 ms / p95 265 ms; lọc nguồn p95 114 ms; lọc chuyên mục p95 49 ms; các loại khác p95 ≤ 34 ms. Khi ép tải, tìm kiếm p95 1,5 s.

Truy vấn đơn lẻ (5 lần, không tải khác; `scripts/vps-test/queries.sh`), trung vị — 1 GB / 2 GB:

| Truy vấn | 1 GB | 2 GB |
|---|---|---|
| Trang chủ, chi tiết bài, quốc gia GB, nguồn, thống kê | 1–3 ms | 1–4 ms |
| Chuyên mục, một feed | 4–7 ms | 5–7 ms |
| Tìm từ không có trong tiêu đề nào (quét toàn bảng) | 94 ms | 96 ms |
| Trang 200 | 127 ms | 122 ms |
| Quốc gia "Chưa xác định" khi không có bài nào | 253 ms | 304 ms |
| Danh sách chuyên mục + chấm đỏ với mốc "đã xem" rất cũ | 544 ms | 486 ms |

Nhận xét:
- **RAM vẫn đủ cho 1 GB**: DB tối đa 238 MiB / 384 MiB, app 11 MiB, 0 OOM. 2 GB **không** cải thiện vì nút thắt là CPU của PostgreSQL (≈ 180% khi ép tải).
- **Độ trễ tăng theo số bài**: các truy vấn phải quét nhiều dòng (tìm kiếm `ILIKE` không có chỉ mục, trang sâu, lọc rỗng, đếm chấm đỏ với mốc cũ) chậm đi tuyến tính. Với 302 nghìn bài vẫn dùng được; với hàng triệu bài (2 năm của 200 feed ≈ 3,4 triệu bài) **chưa đo**, không được suy ra là vẫn nhanh. Cần thêm chỉ mục tìm kiếm (ví dụ `pg_trgm`) hoặc giới hạn thời gian lưu bài trước khi tới quy mô đó.

### 6.3. Worker với 200 feed và 200 website (phân tích từ code, chưa đo)

Không mô phỏng được worker với 200 feed: feed giả lập phải có URL thuộc website thật (allowlist chống SSRF), bật lên sẽ gửi request thật tới báo. Phân tích code (`poll`, `pollSite`, `fetchArticles`, `news.Fetch`):

- Mỗi **website** một goroutine chạy song song; feed trong một website đọc **lần lượt**, cách 1 s, tối đa 30 s/feed; sau đó tối đa 12 trang bài/website/lượt, cách 1,5 s, trong 60 s. Mỗi phản hồi tối đa 4 MiB; HTTP timeout 20 s. Lượt mới chỉ bắt đầu khi lượt cũ xong (không chồng lượt).
- **200 feed / 3 website** (như mô phỏng): website nhiều feed nhất (90) cần ≥ 90 s chỉ cho khoảng nghỉ, cộng thời gian tải và 60 s trang bài → **một lượt ≈ 2,5–3 phút** (ước tính) thay vì 2 phút; mỗi feed được kiểm tra khoảng 3 phút/lần. Sức lấy toàn văn ≈ 12 trang × (1440 phút / 3) ≈ 5 700 trang/website/ngày, cao hơn nhu cầu ước tính (≈ 4 600 bài/ngày cho cả 3 website). RAM worker gần như không tăng theo số feed vì mỗi website chỉ xử lý một phản hồi một lúc: lượt worker thật trên 2 website làm app tăng từ ~9 lên 11–14 MiB.
- **200 website** (khác hẳn): 200 goroutine tải **cùng lúc**, **không có giới hạn song song**; trường hợp xấu mỗi goroutine giữ một phản hồi 4 MiB cộng dữ liệu phân tích → hàng trăm MiB tới hơn 1 GiB trong cùng một lúc (ước tính từ code, chưa đo). Pool DB 10 kết nối sẽ phải xếp hàng. Ngoài ra **app chỉ có 3 adapter**: muốn có 200 website phải viết thêm adapter cho từng website. → **Chưa kiểm chứng** quy mô 200 website; trước khi mở rộng nên thêm giới hạn số website tải song song (ví dụ 8–16) rồi đo lại.

## 7. Dung lượng ổ đĩa (tách khỏi RAM)

| Phần | Số liệu | Nguồn |
|---|---|---|
| Hệ điều hành Ubuntu 24.04 | ≈ 2,5–4 GB | Ước tính (image cloud thông thường); VM Colima tối giản dùng 0,9 GB, không đại diện cho VPS |
| Image Docker | `postgres:17-alpine` 415 MB, app 33 MB (giải nén) | Đo (`docker images`) |
| Image chỉ cần nếu build trên VPS | `golang:1.27-alpine` 377 MB + cache build | Đo; không cần nếu build ở máy khác |
| Database hiện tại | 22 MB cho ~1950 bài (3 website, 19 feed); ≈ 440 bài mới/ngày → ≈ **1,1 GB/năm** | Đo kích thước; tốc độ đo ngày 2026-10-07 |
| Database 200 feed | **7,1 KB/bài** (đo trên 302 nghìn bài) → ≈ 12 GB/năm, ≈ **24 GB sau 2 năm** | Kích thước/bài đo; số bài/ngày là giả định 23 bài/feed/ngày |
| Sao lưu | File `pg_dump -Fc` ≈ 18–19% kích thước database (4,1 MB cho 22 MB) | Đo |
| Log Docker | Lúc đo **không giới hạn**; DB thật ghi 171 KB trong 2,5 ngày. Nay giới hạn 3 × 10 MB/container (≤ 60 MB cho 2 container) | Đo; giới hạn đã thêm vào `compose.yaml` |

| Gói SSD | Quy mô hiện tại (3 website) | 200 feed |
|---|---|---|
| **20 GB** | Đủ nhiều năm: OS + image ≈ 4–5 GB, DB ≈ 1,1 GB/năm, giữ vài bản sao lưu nhỏ | **Không đủ cho 2 năm**: DB đầy ổ trong khoảng 1 năm (≈ 12 GB/năm + sao lưu) |
| **50 GB** | Rất dư | Vừa đủ cho ≈ 2 năm nếu sao lưu cất **ngoài VPS** (DB ≈ 24 GB + 1–2 bản sao lưu ≈ 5 GB/bản + OS/image ≈ 5 GB); nên có chính sách xoá bài cũ |

## 8. Build và triển khai

- **Build trên máy phát triển** (arm64, không cache): 40 s; VM mất tới **≈ 439 MiB** RAM khả dụng trong lúc build (go vet + go test + compile). Trên VPS 1 GB đang chạy app + DB, build tại chỗ có nguy cơ thiếu RAM → **không build trên VPS 1 GB**.
- **Kiến trúc:** máy phát triển và image hiện tại là **arm64**; VPS phổ biến là **amd64** (x86-64). Image arm64 **không chạy được** trên VPS amd64 — không bàn giao image hiện tại cho VPS amd64.
- Thử tạo bản amd64 trên máy này:
  - `docker build --platform linux/amd64` (builder cổ điển, VM không có buildx): **thất bại** ở bước `COPY` ("does not provide the specified platform (linux/amd64)") — giới hạn của builder cổ điển khi build khác kiến trúc, không phải lỗi app.
  - Biên dịch chéo bằng Go (`GOOS=linux GOARCH=amd64 CGO_ENABLED=0`, 12 s): ra file **ELF x86-64, liên kết tĩnh, 12,3 MB**. Chạy thử dưới giả lập QEMU 7.0.0 trong VM: **1/3 lần chạy tốt** (`uname -m` = x86_64, `/healthz` OK, danh sách bài 200), **2/3 lần crash** trong bộ cấp phát bộ nhớ của Go (`fatal error: s.allocCount != s.nelems`, `found bad pointer in Go heap`). Code app không dùng `unsafe` hay cgo; bản arm64 chạy toàn bộ phiên đo không lỗi. Nguyên nhân **nhiều khả năng** do giả lập QEMU user-mode bản cũ, nhưng **chưa xác nhận** — phải chạy bản amd64 trên máy amd64 thật trước khi dùng.
- **Phương án đề xuất** (chưa thực hiện, không push, không deploy):
  1. Build image amd64 trên máy amd64 (CI miễn phí như GitHub Actions, hoặc máy amd64 bất kỳ), hoặc cài `docker buildx` (miễn phí) rồi `docker buildx build --platform linux/amd64`; Dockerfile nên dùng `FROM --platform=$BUILDPLATFORM golang…` + `GOARCH=$TARGETARCH` để biên dịch chéo thay vì giả lập.
  2. Kiểm tra kiến trúc trước khi gửi: `docker image inspect <image> --format '{{.Architecture}}'` phải là `amd64`.
  3. Đưa lên VPS không cần registry: `docker save <image> | gzip | ssh <vps> 'gunzip | docker load'`.
  4. Trên VPS: chạy smoke test (`/healthz`, mở trang, xem log) trước khi chuyển sang dùng thật.
  - VPS 2 GB có thể build tại chỗ (đủ RAM theo số đo), nhưng vẫn nên build sẵn để VPS không phải tải image Go 377 MB.

## 9. Kết luận và đề xuất

### Quy mô hiện tại (3 website, 19 feed, ~2 nghìn bài)
- **VPS 1 GB dùng được**, với điều kiện: (1) không build image trên VPS; (2) bật swap 1 GB làm đệm phòng đỉnh bất thường; (3) giới hạn log Docker (đã có trong `compose.yaml`); (4) reverse proxy nhẹ (Caddy/Nginx); (5) sao lưu chép ra ngoài VPS. Căn cứ: tổng app + DB đo được ≤ ~90 MiB ở mọi mức thử (≤ 14 MiB app, ≤ 77 MiB DB), cách xa giới hạn 576 MiB của ngân sách 1 GB.
- **VPS 2 GB** cho thêm: chỗ cho build tại chỗ, page cache lớn hơn khi dữ liệu tăng, dư cho proxy/giám sát/công cụ khác, ít phụ thuộc swap. Với số đo hiện tại **không cần** để đọc tin nhanh hơn.

### Mục tiêu 200 feed
- RAM: đo với 302 nghìn bài (≈ 2 tháng của 200 feed) vẫn trong ngân sách 1 GB. Cái giới hạn trước là **CPU của PostgreSQL** và **tốc độ tìm kiếm/truy vấn quét bảng** khi số bài tăng; 2 GB RAM không giải quyết được điều đó — cần chỉ mục tìm kiếm, giới hạn thời gian lưu bài, hoặc nhiều vCPU hơn.
- Ổ đĩa: chọn **50 GB** (20 GB không đủ cho 2 năm).
- 200 website: chưa kiểm chứng (cần thêm adapter và giới hạn song song trong worker).

### Đề xuất cấu hình khởi đầu
- VPS 1 vCPU/1 GB hoặc 2 vCPU/2 GB, SSD 20 GB cho quy mô hiện tại (50 GB nếu hướng tới 200 feed), Ubuntu LTS **amd64**, swap file 1 GB.
- **Đã áp dụng vào `compose.yaml` (2026-10-07, sau lượt đo):** `mem_limit` database 384 MB, app 192 MB (đổi bằng `DB_MEM_LIMIT`, `APP_MEM_LIMIT` trong `.env`; VPS 2 GB: 768m/384m) và log Docker tối đa 3 file × 10 MB mỗi container. Không đặt `memswap_limit`: trên máy có swap, Docker cho container dùng thêm swap tối đa bằng `mem_limit` (đệm trước khi OOM); trên máy không swap như VM đo, vượt giới hạn là OOM ngay. Đã kiểm tra trên app local: `docker compose config` hợp lệ, container chạy với đúng giới hạn (DB 34 MiB / 384 MiB, app 9 MiB / 192 MiB sau khởi động), dữ liệu giữ nguyên.
- Giữ `APP_BIND=127.0.0.1`, đặt proxy HTTPS phía trước (README).

### Theo dõi và dấu hiệu cần nâng cấp
| Theo dõi | Lệnh | Dấu hiệu |
|---|---|---|
| RAM, CPU container | `docker stats --no-stream` | App > 100 MiB hoặc DB gần giới hạn kéo dài |
| RAM toàn máy, swap | `free -m` | "available" < 150 MiB hoặc swap dùng liên tục |
| OOM | `dmesg -T \| grep -i -E "oom|killed process"`; `docker inspect -f '{{.State.OOMKilled}} {{.RestartCount}}' news-reader-app-1 news-reader-db-1` | Bất kỳ OOM/restart nào |
| Kích thước DB | `docker compose exec -T db psql -U news -d news -c "SELECT pg_size_pretty(pg_database_size('news'))"` | Vượt 50% ổ đĩa |
| Ổ đĩa | `df -h /`, `docker system df` | Trên 70–80% |
| Tốc độ | `curl -s -o /dev/null -w '%{time_total}\n' 'http://127.0.0.1:8080/api/articles?q=abc'` | Tìm kiếm > 1 s, danh sách > 300 ms thường xuyên |
| Worker | `docker compose logs --since 1h app` | Lượt lấy tin chậm, lỗi lặp lại |

### Phải kiểm lại trên VPS thật
- RAM hệ điều hành thật (`free -m` ngay sau khi cài Docker, trước khi chạy app).
- CPU: chạy lại `scripts/vps-test/run.sh` (ít nhất 20 và 50 phiên) với dữ liệu thật; vCPU VPS chậm hơn M5.
- Image **amd64** chạy ổn định (không có crash như dưới QEMU).
- Proxy HTTPS và RAM của nó.
- Thời gian một lượt worker khi nhiều feed (log + `last_checked`).

## 10. Lệnh tái hiện (đã dùng thật)

Chạy trong thư mục project trên máy phát triển. Các lệnh chỉ đụng tới stack thử `nr-vps-*`, **không** đụng app/DB thật (trừ `pg_dump` chỉ đọc).

```sh
# 1. Sao chép database thật (chỉ đọc) ra file
docker compose exec -T db pg_dump -U news -Fc news > /đường/dẫn/real-copy.dump

# 2. Dựng stack thử với ngân sách 1 GB (đổi 1g thành 2g cho ngân sách 2 GB)
sh scripts/vps-test/stack.sh up 1g /đường/dẫn/real-copy.dump

# 3. Một tình huống: thư mục kết quả, tên, số phiên, số giây (0 phiên = chỉ đo nền)
sh scripts/vps-test/run.sh ketqua idle 0 60
sh scripts/vps-test/run.sh ketqua s20 20 120
THINK=0 sh scripts/vps-test/run.sh ketqua stress20 20 60     # ép tải, không nghỉ

# 4. Một lượt worker thật chỉ trên 2 feed app thật không lấy, rồi tắt lại
docker exec -i nr-vps-db psql -U news -d news -q < scripts/vps-test/crawl-on.sql
docker exec -i nr-vps-db psql -U news -d news -q < scripts/vps-test/crawl-off.sql

# 5. Sao lưu trong lúc có tải (chạy song song với bước 3)
docker exec nr-vps-db pg_dump -U news -Fc news > /đường/dẫn/backup-test.dump

# 6. Đổi ngân sách mà giữ dữ liệu, rồi khởi động lạnh
docker update --memory 768m --memory-swap 768m nr-vps-db
docker update --memory 384m --memory-swap 384m nr-vps-app
docker stop nr-vps-app nr-vps-db && docker start nr-vps-db && docker start nr-vps-app

# 7. Dữ liệu 200 feed (chỉ stack thử) và đo truy vấn
docker exec -i nr-vps-db psql -U news -d news -v articles=300000 < scripts/vps-test/seed-200.sql
sh scripts/vps-test/queries.sh

# 8. Đo build (RAM VM xem bằng: colima ssh -- grep MemAvailable /proc/meminfo)
docker build --no-cache -t nr-vps-build:arm64 .
docker run --rm -v "$PWD":/src -w /src -e CGO_ENABLED=0 -e GOOS=linux -e GOARCH=amd64 \
  golang:1.27-alpine3.24 go build -trimpath -ldflags='-s -w' -o /src/tmp/vps-amd64/server ./cmd/server

# 9. Dọn
sh scripts/vps-test/stack.sh down
```

Giải thích ngắn: `stack.sh` tạo database và app thử riêng có giới hạn RAM; `run.sh` vừa tạo tải (từ máy Mac) vừa ghi RAM/CPU mỗi giây từ kernel, cuối cùng in một dòng tóm tắt; `summarize.mjs` tính đỉnh/trung bình; `queries.sh` đo từng truy vấn. Kết quả tóm tắt của lượt đo này: `scripts/vps-test/ket-qua-2026-10-07/`.

## 11. Những gì chưa đo được

- Mọi thứ trên **VPS Linux thật**: CPU, RAM hệ điều hành, đĩa, mạng, swap (VM đo không có swap; mọi container đặt `--memory-swap` = `--memory`).
- Reverse proxy HTTPS.
- Worker với 200 feed và với 200 website (mục 6.3 là phân tích code).
- Tìm kiếm/truy vấn với hàng triệu bài (chỉ đo tới 302 nghìn).
- Image amd64 chạy ổn định trên máy amd64 thật.
- Người dùng từ nhiều IP thật (giả lập dùng 1 IP nên phần lượt đọc/heartbeat bị giới hạn tần suất).
- Thời lượng đo mỗi mức là 2 phút; chưa chạy dài (nhiều giờ/ngày) để xem rò rỉ bộ nhớ — app thật đã chạy liên tục ~1 giờ sau lần cập nhật cuối ở 9 MiB.
