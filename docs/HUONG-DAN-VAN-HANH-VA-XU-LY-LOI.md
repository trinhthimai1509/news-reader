# Hướng dẫn vận hành và xử lý lỗi — News Reader

Tài liệu này dành cho người vận hành có kiến thức kỹ thuật hạn chế. Mục tiêu: tự khởi động, kiểm tra, xác định nhóm lỗi, thao tác an toàn và gửi đủ thông tin cho người hỗ trợ (hoặc AI) khi cần.

Đọc kèm: [GIAI-THICH-HE-THONG-VA-KY-THUAT.md](GIAI-THICH-HE-THONG-VA-KY-THUAT.md) (giải thích hệ thống hoạt động thế nào).

---

## 0. Cách đọc tài liệu này

### 0.1. Nhãn mức an toàn

Mỗi lệnh hoặc thao tác có một nhãn:

| Nhãn | Ý nghĩa | Có cần sao lưu trước? |
|---|---|---|
| **[CHỈ ĐỌC]** | Chỉ xem, không thay đổi gì. Chạy bao nhiêu lần cũng được. | Không |
| **[THAY ĐỔI TRẠNG THÁI]** | Dừng/chạy lại chương trình, đổi cài đặt. Không xoá dữ liệu, có thể làm lại hoặc hoàn tác. | Không bắt buộc |
| **[THAY ĐỔI DỮ LIỆU]** | Ghi vào cơ sở dữ liệu (thêm/xoá nguồn, đổi mật khẩu, migration). | Nên sao lưu |
| **[NGUY HIỂM — CÓ THỂ MẤT DỮ LIỆU]** | Xoá dữ liệu hoặc ghi đè cơ sở dữ liệu. Không hoàn tác được nếu không có bản sao lưu. | **Bắt buộc**, và phải xác nhận riêng với người chịu trách nhiệm |

### 0.2. Nhãn mức xác minh

| Nhãn | Ý nghĩa |
|---|---|
| **Đã dùng trong project** | Lệnh này đã được chạy thật trên máy phát triển trong quá trình làm project (macOS + Docker qua Colima). |
| **Chưa kiểm chứng** | Lệnh hợp lý theo cấu hình hiện có nhưng **chưa được chạy thử** trong project. Làm cùng người hỗ trợ, đọc kỹ điều kiện. |

### 0.3. Thuật ngữ dùng nhiều nhất (giải thích ngắn)

| Thuật ngữ | Nghĩa đời thường |
|---|---|
| **Terminal** | Cửa sổ gõ lệnh bằng chữ (macOS: ứng dụng *Terminal*; Windows: *PowerShell* hoặc *Windows Terminal*). |
| **Docker** | Phần mềm chạy chương trình trong các "hộp" riêng biệt (gọi là *container*), để không phải cài từng thành phần lên máy. |
| **Container** | Một "hộp" đang chạy. Project có 2 container: `app` (chương trình chính) và `db` (cơ sở dữ liệu). |
| **Image** | "Bản đóng gói" để tạo container. Khi sửa code phải *build* (đóng gói) lại image. |
| **Volume** | Ổ lưu dữ liệu của Docker. Dữ liệu tin tức nằm trong volume tên `pgdata` (tên đầy đủ thường là `news-reader_pgdata`). Xoá volume = mất dữ liệu. |
| **Docker Compose** | Công cụ chạy nhiều container cùng lúc theo file `compose.yaml`. Các lệnh bắt đầu bằng `docker compose`. |
| **PostgreSQL** | Phần mềm cơ sở dữ liệu, nơi lưu nguồn tin, bài viết, tài khoản quản trị. |
| **Log** | Nhật ký chương trình ghi ra khi chạy: lỗi, cảnh báo, sự kiện. |
| **Worker** | Phần chạy nền bên trong `app`, tự động đi lấy tin định kỳ. |
| **RSS / feed** | Một địa chỉ mà báo cung cấp danh sách bài mới dưới dạng máy đọc được. |
| **Migration** | Đoạn lệnh nâng cấp cấu trúc cơ sở dữ liệu (thêm bảng, thêm cột). Tự chạy khi `app` khởi động. |
| **Backup / restore** | Sao lưu dữ liệu ra file / khôi phục dữ liệu từ file. |
| **Cổng (port)** | "Số cửa" mà chương trình lắng nghe. App dùng cổng `8080`. |
| **VPS** | Máy chủ thuê trên Internet. Project **chưa được triển khai trên VPS** (xem mục A.13). |

---

## A. Bắt đầu sử dụng

### A.1. Những thứ cần có để chạy trên máy cá nhân (local)

| Cần có | Ghi chú |
|---|---|
| Docker và Docker Compose | macOS: Docker Desktop hoặc Colima (máy phát triển dùng Colima). Windows/Linux: Docker Desktop hoặc Docker Engine. Kiểm tra bằng `docker compose version`. |
| Thư mục mã nguồn project | Có các file `compose.yaml`, `Dockerfile`, thư mục `cmd/`, `web/`, `migrations/`. |
| Trình duyệt | Chrome, Edge, Firefox, Safari bản mới. |
| Kết nối Internet | Để worker lấy tin và để trình duyệt tải ảnh/video từ máy chủ của báo. |
| Công cụ tạo mật khẩu ngẫu nhiên (khuyến nghị) | Ví dụ `openssl` (có sẵn trên macOS/Linux). |

**Không cần** cài Go, Node.js hay PostgreSQL lên máy: Docker lo phần đó.

### A.2. Mở terminal đúng thư mục

**Làm ở đâu:** máy cá nhân.
**Điều kiện:** biết thư mục chứa project.

1. Mở Terminal.
2. Gõ `cd ` (có dấu cách), kéo thả thư mục project vào cửa sổ terminal (macOS tự điền đường dẫn), nhấn Enter.
3. Kiểm tra **[CHỈ ĐỌC]**:
   ```sh
   ls compose.yaml Dockerfile
   ```
**Kết quả mong đợi:** in ra `compose.yaml Dockerfile`.
**Nếu khác** (`No such file or directory`): bạn đang ở sai thư mục. Lặp lại bước 2.

> Mọi lệnh `docker compose …` trong tài liệu này phải chạy **trong thư mục project**.

### A.3. Khởi động lần đầu

**Làm ở đâu:** máy cá nhân, thư mục project. **Mức:** [THAY ĐỔI TRẠNG THÁI]. **Xác minh:** Đã dùng trong project.

**Điều kiện:** Docker đang chạy (Docker Desktop đã mở, hoặc `colima start` đã chạy).

1. Tạo file cấu hình từ mẫu (chỉ làm lần đầu, **không** làm lại nếu đã có `.env`):
   ```sh
   cp .env.example .env
   ```
2. Tạo mật khẩu cơ sở dữ liệu ngẫu nhiên:
   ```sh
   openssl rand -hex 32
   ```
   Mở file `.env` bằng trình soạn thảo văn bản, thay giá trị sau `POSTGRES_PASSWORD=` bằng chuỗi vừa tạo. Dùng chuỗi hex (chỉ chữ số và a–f) vì nó được ghép vào địa chỉ kết nối cơ sở dữ liệu.
3. Build và chạy:
   ```sh
   docker compose up --build -d
   ```
   Lần đầu mất vài phút (tải image, chạy kiểm tra `go vet` và `go test` trong lúc build).

**Kết quả mong đợi:** dòng cuối có `Container news-reader-app-1 Started` (tên có thể khác chút tuỳ thư mục).
**Nếu khác:**
- `Set POSTGRES_PASSWORD in .env` → chưa đặt mật khẩu ở bước 2.
- `Cannot connect to the Docker daemon` → Docker chưa chạy. Mở Docker Desktop hoặc chạy `colima start`.
- Lỗi khi build (`FAIL`, `go vet`) → code có lỗi, nhờ lập trình viên (mục E).

> **Lưu ý về `.env`:** file này chứa mật khẩu. Không gửi cho người khác, không đưa lên Git (đã có trong `.gitignore`), không dán vào AI.

### A.4. Kiểm tra trạng thái

**Mức:** [CHỈ ĐỌC]. **Xác minh:** Đã dùng trong project.

```sh
docker compose ps
```

**Kết quả tốt:** hai dòng `app` và `db`, cột STATUS có `Up … (healthy)`. Cột PORTS của app là `127.0.0.1:8080->8080/tcp`.

| STATUS thấy được | Nghĩa | Làm gì |
|---|---|---|
| `Up … (healthy)` | Đang chạy bình thường | Không cần làm gì |
| `Up … (health: starting)` | Vừa khởi động | Đợi 30 giây, kiểm tra lại |
| `Up … (unhealthy)` | Chạy nhưng tự kiểm tra thất bại (thường do không kết nối được database) | Xem mục C.2 |
| `Restarting` | Bị lỗi, đang tự khởi động lại liên tục | Xem log (A.6), mục C.1/C.3 |
| `Exited` hoặc không có dòng `app` | Đã dừng | `docker compose up -d`, rồi xem log |

Kiểm tra thêm bằng địa chỉ sức khoẻ **[CHỈ ĐỌC]**:
```sh
curl -s http://127.0.0.1:8080/healthz
```
**Tốt:** `{"status":"ok"}`. **Xấu:** `{"error":"database unavailable"}` (app chạy nhưng không nói chuyện được với database → C.2) hoặc `Connection refused` (app không chạy hoặc sai cổng → C.1).

### A.5. Mở website

Trên **chính máy đang chạy Docker**, mở trình duyệt: **http://127.0.0.1:8080**

**Kết quả mong đợi:** trang "Tin mới" với danh sách bài, thanh chuyên mục, ô "Quốc gia nguồn", "Nguồn", "Tìm tiêu đề". Góc trên có nút "Đọc tin" và "Đăng nhập quản trị". **Không** cần nhập mã hay đăng nhập để đọc.

Lần chạy đầu tiên danh sách có thể trống vài phút: worker đang lấy tin lần đầu.

### A.6. Xem log có giới hạn

**Mức:** [CHỈ ĐỌC]. **Xác minh:** Đã dùng trong project.

Không cần đọc toàn bộ log. Chỉ xem phần cuối hoặc khoảng thời gian gần đây:

```sh
docker compose logs --tail 100 app          # 100 dòng cuối của app
docker compose logs --since 30m app         # log 30 phút gần nhất
docker compose logs --tail 50 db            # 50 dòng cuối của database
docker compose logs --since 1h app | grep -i -E "error|fatal|panic|migration|429"   # chỉ dòng đáng chú ý (macOS/Linux)
```

Theo dõi trực tiếp (nhấn `Ctrl + C` để thoát, không làm dừng app):
```sh
docker compose logs -f --tail 20 app
```

Các dòng log **bình thường** khi khởi động:

| Dòng log | Nghĩa |
|---|---|
| `COOKIE_SECURE=false: the admin cookie is sent over plain HTTP…` | Nhắc nhở: đang chạy HTTP (bình thường khi chạy local). Phải đổi khi dùng HTTPS (A.13). |
| `migration applied: 0xx_….sql` | Lần đầu áp dụng một bản nâng cấp database. Chỉ xuất hiện một lần cho mỗi file. |
| `No admin account yet: …` | Chưa tạo tài khoản quản trị (A.10). Đọc tin vẫn bình thường. |
| `Personal News Reader listening on :8080` | App đã sẵn sàng. |
| `admin login from <địa chỉ>` | Có người đăng nhập quản trị thành công. |
| `ADMIN_TOKEN is no longer used…` / `LOCAL_NO_AUTH is no longer used…` | `.env` còn biến của bản cũ; vô hại, có thể xoá dòng đó khỏi `.env`. (Với `compose.yaml` hiện tại hai biến này không được chuyển vào container, nên thường không thấy dòng này.) |

### A.7. Dừng và khởi động lại

| Mục đích | Lệnh | Mức | Dữ liệu |
|---|---|---|---|
| Dừng tất cả, giữ dữ liệu | `docker compose stop` | [THAY ĐỔI TRẠNG THÁI] | Giữ nguyên |
| Chạy lại sau khi dừng | `docker compose start` | [THAY ĐỔI TRẠNG THÁI] | Giữ nguyên |
| Khởi động lại chỉ app | `docker compose restart app` | [THAY ĐỔI TRẠNG THÁI] | Giữ nguyên |
| Áp dụng thay đổi trong `.env` | `docker compose up -d app` | [THAY ĐỔI TRẠNG THÁI] | Giữ nguyên |
| Áp dụng thay đổi code (build lại) | `docker compose up --build -d` | [THAY ĐỔI TRẠNG THÁI] (có thể kèm migration = [THAY ĐỔI DỮ LIỆU]) | Giữ nguyên; sao lưu trước nếu có migration mới |
| Xoá container, giữ dữ liệu | `docker compose down` | [THAY ĐỔI TRẠNG THÁI] | Giữ nguyên (volume không bị xoá) |

`docker compose restart app` **không** đọc lại `.env`. Sau khi sửa `.env`, dùng `docker compose up -d app`.

### A.8. Phân biệt restart, rebuild, migration và khôi phục dữ liệu

| Thao tác | Nó làm gì | Khi nào dùng | Ảnh hưởng dữ liệu |
|---|---|---|---|
| **Restart** | Tắt rồi bật lại chương trình đang có | App treo, sau sự cố mạng | Không |
| **Rebuild** | Đóng gói lại chương trình từ mã nguồn mới, tạo container mới | Sau khi cập nhật code | Không trực tiếp; nhưng code mới có thể kèm migration |
| **Migration** | Nâng cấp cấu trúc database (thêm bảng/cột). Tự chạy khi app khởi động, mỗi file một lần, ghi lại trong bảng `schema_migrations` | Tự động, không cần gọi tay | **Có** thay đổi cấu trúc; các migration hiện có chỉ *thêm*, không xoá bài. Vẫn phải sao lưu trước khi cập nhật |
| **Khôi phục (restore)** | Đưa dữ liệu từ file sao lưu vào database | Mất dữ liệu, hỏng dữ liệu, cập nhật lỗi | **Ghi đè** dữ liệu hiện tại nếu restore vào database thật. Xem mục D |

### A.9. Lệnh nào giữ dữ liệu, lệnh nào có thể xoá dữ liệu

| Lệnh | Kết quả với dữ liệu |
|---|---|
| `docker compose ps`, `logs`, `stop`, `start`, `restart`, `up -d`, `up --build -d`, `down` | **Giữ** dữ liệu |
| `docker compose down -v` | **[NGUY HIỂM — CÓ THỂ MẤT DỮ LIỆU]** Xoá volume `pgdata` ⇒ **mất toàn bộ** nguồn, bài, tài khoản quản trị. |
| `docker volume rm news-reader_pgdata` | **[NGUY HIỂM — CÓ THỂ MẤT DỮ LIỆU]** Như trên. |
| `docker system prune --volumes`, `docker volume prune` | **[NGUY HIỂM — CÓ THỂ MẤT DỮ LIỆU]** Có thể xoá volume không gắn với container đang chạy (ví dụ khi app đang dừng). |
| `colima delete` (macOS dùng Colima) | **[NGUY HIỂM — CÓ THỂ MẤT DỮ LIỆU]** Xoá cả máy ảo chứa Docker, kèm mọi volume. |

> **Quy tắc:** chỉ chạy các lệnh trong nhóm nguy hiểm khi đã có bản sao lưu mới, đã kiểm tra file sao lưu (D.3), và người chịu trách nhiệm đã đồng ý **riêng cho lần đó**.

### A.10. Tài khoản quản trị: tạo lần đầu, đổi/reset mật khẩu

Hệ thống có **đúng một** tài khoản quản trị. **Không có tài khoản hay mật khẩu mặc định.** Khi chưa tạo, mọi người vẫn đọc tin được nhưng không ai vào được màn hình "Nguồn tin".

**Làm ở đâu:** máy đang chạy Docker, thư mục project. **Điều kiện:** container `app` đang chạy (`docker compose ps`).

| Việc | Lệnh | Mức | Xác minh |
|---|---|---|---|
| Xem đã có tài khoản chưa | `docker compose exec app /app/server admin status` | [CHỈ ĐỌC] (lệnh có kiểm tra migration, nhưng không thay đổi gì khi đã cập nhật) | Đã dùng trong project |
| Tạo tài khoản đầu tiên | `docker compose exec app /app/server admin create <tên-đăng-nhập>` | [THAY ĐỔI DỮ LIỆU] | Đã dùng trong project (cách nhập qua stdin) |
| Đổi / quên mật khẩu | `docker compose exec app /app/server admin reset-password` | [THAY ĐỔI DỮ LIỆU] — huỷ mọi phiên đăng nhập | Đã kiểm bằng test tự động |
| Đăng xuất mọi thiết bị | `docker compose exec app /app/server admin logout-all` | [THAY ĐỔI DỮ LIỆU] | Đã kiểm bằng test tự động |

**Quy tắc:**
- Tên đăng nhập: 3–64 ký tự, chỉ chữ cái không dấu, số, dấu `.` `_` `-`.
- Mật khẩu: 12–128 ký tự. Khi có terminal, chương trình hỏi **2 lần** và **không hiện ký tự** khi gõ (bình thường, cứ gõ rồi Enter).
- Mật khẩu chỉ được lưu dạng "băm" (biến đổi một chiều, không giải ngược được). Quên mật khẩu thì **đặt mật khẩu mới**, không xem lại được mật khẩu cũ.

**Kết quả mong đợi:**
- `create`: `Đã tạo tài khoản quản trị "<tên>".`
- `reset-password`: `Đã đổi mật khẩu của "<tên>"; N phiên đăng nhập đã bị huỷ.`
- `status`: `Tài khoản quản trị: "<tên>"; phiên chưa hết hạn: N.` hoặc `Chưa có tài khoản quản trị.`

**Nếu khác:**

| Thông báo | Nghĩa | Làm gì |
|---|---|---|
| `Lỗi: đã có tài khoản quản trị; dùng server admin reset-password…` | Đã có tài khoản, chỉ được một | Dùng `admin status` để xem tên, rồi `reset-password` nếu quên mật khẩu |
| `Lỗi: chưa có tài khoản quản trị; dùng server admin create…` | Gọi reset khi chưa có tài khoản | Dùng `create` |
| `Lỗi: mật khẩu cần từ 12 đến 128 ký tự` | Mật khẩu quá ngắn/dài | Chạy lại với mật khẩu khác |
| `Lỗi: hai lần nhập không khớp` | Gõ khác nhau | Chạy lại |
| `Lỗi: dùng: server admin create <tên-đăng-nhập> (3–64 ký tự…)` | Tên không hợp lệ | Chọn tên khác |
| `service "app" is not running` | Container app chưa chạy | `docker compose up -d`, rồi thử lại |

**Khi không có terminal tương tác** (ví dụ chạy bằng script): mật khẩu được đọc từ **dòng đầu tiên** của dữ liệu đưa vào. Cách an toàn:
```sh
# Tạo file chỉ mình bạn đọc được, ghi mật khẩu vào dòng đầu (bằng trình soạn thảo), rồi:
docker compose exec -T app /app/server admin reset-password < duong-dan/file-mat-khau
# Xoá file ngay sau đó.
```
**Không** viết mật khẩu trực tiếp trong lệnh (ví dụ `echo matkhau | …`): nó bị lưu vào lịch sử terminal.

**Chạy không dùng Docker (dành cho lập trình viên):** `go run ./cmd/server admin create <tên>` từ thư mục gốc project, cần biến `DATABASE_URL`.

### A.11. Sử dụng website

#### Đọc tin (ai cũng dùng được, không cần đăng nhập)
- **Chuyên mục:** bấm các nút "Thời sự", "Thế giới"… "Tất cả" là mọi chuyên mục.
- **Quốc gia nguồn:** chọn quốc gia của **tờ báo** (không phải quốc gia được nhắc trong bài). Danh sách chỉ hiện các nước đang có nguồn, thêm "Chưa xác định" nếu có website chưa được gán quốc gia.
- **Nguồn:** chọn một feed cụ thể. Khi đã chọn quốc gia, danh sách nguồn chỉ còn nguồn của nước đó.
- **Tìm tiêu đề:** gõ từ khoá rồi nhấn Enter hoặc "Tìm". Tìm theo tiêu đề, không phân biệt hoa/thường. Xoá hết chữ trong ô sẽ tự quay lại danh sách không lọc từ khoá.
- Các bộ lọc kết hợp được với nhau. Đổi bộ lọc thì quay về trang 1. Mỗi trang 30 bài.
- **Đọc bài:** bấm "Đọc bài". Trang bài luôn có tên nguồn, quốc gia, chuyên mục, giờ đăng và nút "Đọc bài gốc tại …".
- **Chấm đỏ:** có bài mới kể từ lần cuối bạn mở chuyên mục đó **trên trình duyệt này** (giải thích ở C.15).

#### Đăng nhập và đăng xuất quản trị
1. Bấm **"Đăng nhập quản trị"** (góc trên phải).
2. Nhập tên đăng nhập và mật khẩu đã tạo ở A.10, bấm "Đăng nhập".
3. Thành công: mở màn hình "Quản lý nguồn"; góc trên có "Nguồn tin" và "Đăng xuất".
4. Đăng xuất: bấm **"Đăng xuất"**. Phiên bị huỷ ở máy chủ.

Phiên tự hết hạn sau **12 giờ** kể từ lúc đăng nhập, hoặc sau **2 giờ** không thao tác (mặc định; đổi bằng `ADMIN_SESSION_TTL`, `ADMIN_SESSION_IDLE` trong `.env`, rồi `docker compose up -d app`).

#### Quản lý nguồn (chỉ quản trị viên)
| Việc | Cách làm |
|---|---|
| Thêm nguồn | Dán URL vào ô "URL nguồn tin", bấm "Thêm nguồn". Chấp nhận: trang chủ, RSS hoặc trang chuyên mục **đã có ánh xạ** của VnExpress, BBC News, Tuổi Trẻ. Ví dụ: `https://vnexpress.net/rss/thoi-su.rss`, `https://feeds.bbci.co.uk/news/world/rss.xml`, `https://tuoitre.vn/thoi-su.rss`, hoặc `https://vnexpress.net/`. |
| Đổi tên hiển thị | Sửa ô "Tên hiển thị" trên thẻ nguồn, bấm "Lưu tên". |
| Đổi chuyên mục của feed | Chọn trong ô "Chuyên mục" trên thẻ nguồn. Mọi bài feed đó đã liệt kê sẽ theo chuyên mục mới. |
| Tắt / bật nguồn | "Tắt nguồn": ngừng lấy tin mới, bài cũ vẫn đọc được. "Bật nguồn": lấy tin trở lại. |
| Xoá nguồn | "Xoá nguồn" (có hộp xác nhận): ẩn nguồn **và các bài nó đã lấy**. Đây là xoá mềm: thêm lại cùng URL sẽ khôi phục nguồn và bài. Bài cũng thuộc một feed khác đang bật sẽ chuyển sang feed đó ở lượt lấy tin sau. |
| Chọn quốc gia | Khung **"Quốc gia của website"**: chọn quốc gia cho từng website (BBC News, Tuổi Trẻ Online, VnExpress). Áp dụng cho **mọi feed** của website đó. Chọn "Chưa xác định" để bỏ gán. |
| Xem tiến độ bổ sung ảnh/tác giả | Dòng thông báo xanh "Bổ sung tác giả, ảnh và video cho bài đã lưu: … bài xong, … bài đang chờ". |
| Xem lỗi của từng feed | Thẻ nguồn hiện "Kiểm tra gần nhất / Thành công" và dòng lỗi màu đỏ nếu lần lấy gần nhất thất bại. |

### A.12. localhost, 127.0.0.1 và IP VPS: dùng địa chỉ nào ở đâu

| Địa chỉ | Nghĩa | Dùng được từ đâu |
|---|---|---|
| `http://127.0.0.1:8080` | "Chính máy này", cổng 8080 | Chỉ trên **máy đang chạy Docker** |
| `http://localhost:8080` | Tên gọi khác của 127.0.0.1 | Chỉ trên máy đang chạy Docker |
| `http://<IP-LAN>:8080` (ví dụ 192.168.x.x) | Địa chỉ máy trong mạng nhà/công ty | **Không vào được** với cấu hình hiện tại (cố ý: app chỉ mở trên 127.0.0.1) |
| `http://<IP-VPS>:8080` | Địa chỉ công khai của VPS | **Không vào được** với cấu hình hiện tại, và không nên mở thẳng (không có HTTPS) |
| `https://<tên-miền>` | Tên miền trỏ tới VPS, qua reverse proxy HTTPS | **Chưa triển khai** (A.13) |

**Lưu ý quan trọng:** `127.0.0.1` trên điện thoại là **chính điện thoại**, không phải máy tính của bạn. Muốn xem trên điện thoại cần triển khai qua HTTPS (chưa làm).

Ghi chú: trình duyệt coi `127.0.0.1:8080` và `localhost:8080` là **hai trang khác nhau**. Vì vậy trạng thái "đã xem" (chấm đỏ) và phiên đăng nhập ở hai địa chỉ này tách biệt nhau. Nên dùng cố định một địa chỉ.

### A.13. Triển khai lên VPS — trạng thái hiện tại

**Trạng thái: CHƯA TRIỂN KHAI, CHƯA KIỂM THỬ trên VPS hay qua HTTPS.** Phần dưới chỉ mô tả những gì repo đã chuẩn bị và các việc cần làm; các lệnh đánh dấu "Chưa kiểm chứng".

**Repo đã có sẵn:**
- `compose.yaml` chỉ mở cổng app trên `127.0.0.1` (biến `APP_BIND`, mặc định `127.0.0.1`), PostgreSQL không mở cổng ra ngoài.
- Container app chạy bằng user không phải root, hệ thống file chỉ đọc, bỏ mọi quyền đặc biệt (`read_only`, `cap_drop: [ALL]`, `no-new-privileges`).
- Biến `COOKIE_SECURE=true` để cookie đăng nhập chỉ đi qua HTTPS.

**Repo chưa có:** cấu hình reverse proxy (Caddy/Nginx), cấu hình tường lửa, script sao lưu tự động, giám sát/cảnh báo, giới hạn RAM/CPU cho container.

**Hai cách truy cập từ xa (cả hai: Chưa kiểm chứng):**

1. **SSH tunnel (đường hầm SSH)** — đơn giản nhất cho một người dùng, không cần tên miền:
   ```sh
   # Chạy trên MÁY CÁ NHÂN (không phải trên VPS). Chưa kiểm chứng.
   ssh -L 8080:127.0.0.1:8080 <user>@<IP-VPS>
   ```
   Giữ cửa sổ đó mở, rồi trên máy cá nhân mở `http://127.0.0.1:8080`. Cookie không cần `Secure` vì kết nối nằm trong SSH.

2. **Reverse proxy HTTPS** (Caddy hoặc Nginx trên VPS, có tên miền và chứng chỉ TLS). Danh sách việc cần làm (Chưa kiểm chứng):
   - Giữ `APP_BIND=127.0.0.1`; proxy nối tới `127.0.0.1:8080`. **Không** đặt `APP_BIND=0.0.0.0`.
   - Đặt `COOKIE_SECURE=true` trong `.env`, rồi `docker compose up -d app`. Cookie đổi tên thành `__Host-nr_admin`.
   - Proxy phải giữ nguyên header `Host` và `Origin` (app kiểm tra chống CSRF bằng cách so hai giá trị này).
   - Đặt giới hạn tần suất cho `POST /api/admin/login` ở proxy: sau proxy mọi request có cùng địa chỉ nguồn, nên giới hạn đăng nhập theo địa chỉ của app thành giới hạn chung (xem C.7).
   - Mở tường lửa chỉ cổng 22 (SSH), 80, 443.
   - Thiết lập sao lưu theo lịch và lưu bản sao ra ngoài VPS (mục D).
   - Kiểm tra sau triển khai: đọc tin, đăng nhập, cookie có `Secure`, đăng xuất, thử request ghi từ trang khác bị từ chối.

**Lệnh trên VPS:** các lệnh `docker compose …` trong tài liệu này giống hệt khi chạy trên VPS (Linux), nhưng phải chạy **trong phiên SSH vào VPS**, trong thư mục project trên VPS. Lệnh `curl http://127.0.0.1:8080/...` chạy trên VPS kiểm tra app trên VPS; chạy trên máy cá nhân thì kiểm tra app trên máy cá nhân.

---

## B. Quy trình điều tra lỗi

Đi theo thứ tự từ **ít tác động nhất** đến nhiều hơn. Phần lớn bước chỉ đọc. Dừng ở bước tìm ra nguyên nhân.

```mermaid
flowchart TD
    S1[1. Ghi lại hiện tượng và thời điểm] --> S2[2. Trình duyệt, mạng, bộ lọc]
    S2 -->|Vẫn lỗi| S3[3. App và container]
    S3 -->|App chạy tốt| S4[4. PostgreSQL]
    S4 -->|Database tốt| S5[5. Worker]
    S5 -->|Worker chạy| S6[6. RSS, trang gốc, CDN]
    S6 --> S7[7. Phân loại: ứng dụng, cấu hình, hạ tầng hay giới hạn nguồn]
    S2 -->|Hết lỗi| OK[Ghi lại nguyên nhân, kết thúc]
    S3 -->|Tìm thấy| FIX[Tra mục C tương ứng]
    S4 -->|Tìm thấy| FIX
    S5 -->|Tìm thấy| FIX
    S6 -->|Tìm thấy| FIX
```

### Bước 1. Ghi lại hiện tượng và thời điểm [CHỈ ĐỌC]

Ghi vào một file văn bản (dùng lại cho mẫu báo lỗi ở mục E):
- Thời điểm (ngày, giờ, múi giờ).
- Địa chỉ đang mở (`http://127.0.0.1:8080`, …) và màn hình (danh sách, đọc bài, quản lý nguồn).
- Đã làm gì ngay trước đó (bấm nút nào, nhập URL nào).
- Thông báo lỗi hiện trên màn hình (**chép nguyên văn** hoặc chụp màn hình).
- Lỗi xảy ra một lần hay lặp lại; một bài/nguồn hay tất cả.

**Bước tiếp theo:** bước 2.

### Bước 2. Kiểm tra trình duyệt, mạng và bộ lọc [CHỈ ĐỌC / THAY ĐỔI TRẠNG THÁI nhẹ]

| Kiểm tra | Cách làm | Tốt | Xấu → làm gì |
|---|---|---|---|
| Đúng địa chỉ | Thanh địa chỉ là `http://127.0.0.1:8080` (hoặc địa chỉ đã thống nhất) | Trang mở | Sai cổng/địa chỉ → C.1 |
| Bộ lọc đang bật | Xem ô Quốc gia nguồn, Nguồn, Tìm tiêu đề, chuyên mục | Đều "Tất cả" | Đặt lại về "Tất cả" rồi xem lại |
| Tải lại trang | Bấm "Tải lại" trên trang; rồi tải lại cả trang (Cmd+Shift+R trên Mac, Ctrl+Shift+R trên Windows) | Hết lỗi | Còn lỗi → tiếp |
| Trình duyệt khác / cửa sổ ẩn danh | Mở cùng địa chỉ | Chạy được ở đó | Lỗi do cache/tiện ích trình duyệt → C.16 |
| Internet | Mở một trang web khác | Mở được | Mất mạng: ảnh và tin mới không tải được, chờ mạng |

**Bước tiếp theo:** nếu vẫn lỗi, bước 3.

### Bước 3. Kiểm tra app và container [CHỈ ĐỌC]

```sh
docker compose ps
curl -s http://127.0.0.1:8080/healthz
docker compose logs --tail 50 app
```

| Thấy gì | Kết luận | Tiếp theo |
|---|---|---|
| app `Up (healthy)`, healthz `{"status":"ok"}`, log không có lỗi mới | App và kết nối database tốt | Bước 4 (nếu lỗi liên quan dữ liệu) hoặc bước 5 (nếu thiếu bài mới) |
| app `Restarting`/`Exited` | App không khởi động được | Đọc 50 dòng log cuối, tìm dòng có `migration:`, `must`, `invalid`, `merge duplicates` → C.1, C.3 |
| healthz `database unavailable` | App chạy, database có vấn đề | Bước 4, C.2 |
| `Connection refused` | App không lắng nghe ở cổng này | C.1 |
| `Cannot connect to the Docker daemon` | Docker chưa chạy | Mở Docker Desktop / `colima start` |

### Bước 4. Kiểm tra PostgreSQL [CHỈ ĐỌC]

```sh
docker compose exec db pg_isready -U news -d news
docker compose exec db psql -U news news -c "SELECT count(*) AS bai, max(fetched_at) AS lan_nhan_gan_nhat FROM articles;"
docker system df
df -h
```

| Thấy gì | Kết luận | Tiếp theo |
|---|---|---|
| `accepting connections`; câu lệnh trả số bài và thời gian gần đây | Database tốt | Bước 5 |
| `no response` hoặc container db không `healthy` | Database không chạy | `docker compose logs --tail 50 db`, C.2 |
| `df -h` báo ổ đĩa gần 100% (cột Use%) | Hết dung lượng | C.2 |
| `max(fetched_at)` cũ hơn nhiều giờ | Không nhận bài mới | Bước 5 |

> Chỉ chạy câu lệnh `SELECT` (đọc). Không chạy `UPDATE`, `DELETE`, `DROP` nếu không có hướng dẫn cụ thể từ người hỗ trợ và bản sao lưu.

### Bước 5. Kiểm tra worker [CHỈ ĐỌC]

Worker chạy bên trong container `app`, lấy tin khi app khởi động, rồi mặc định mỗi 2 phút (`POLL_INTERVAL`).

**Cách dễ nhất:** đăng nhập quản trị, mở "Nguồn tin", xem từng thẻ nguồn: "Kiểm tra gần nhất", "Thành công", dòng lỗi đỏ.

**Cách bằng lệnh:**
```sh
docker compose exec db psql -U news news -c "SELECT id, name, enabled, deleted, last_checked, last_success, left(last_error,120) AS loi FROM sources WHERE NOT deleted ORDER BY adapter, id;"
docker compose logs --since 30m app | grep -E "^.*(source [0-9]+|articles:)" | tail -20
```

| Thấy gì | Kết luận | Tiếp theo |
|---|---|---|
| `last_checked` trong vài phút gần đây, `loi` trống | Worker chạy, nguồn trả lời bình thường | Nếu vẫn thiếu bài: nguồn chậm (C.10) |
| `enabled = f` | Nguồn đang tắt | Bật lại trong màn hình Nguồn tin |
| `last_checked` cũ hàng giờ với mọi nguồn | Worker không chạy hoặc bị kẹt | Xem log, thử `docker compose restart app` (C.10) |
| `loi` có `HTTP 429` hoặc `HTTP 503` | Nguồn yêu cầu giảm tần suất | Chờ; xem C.10 |
| `loi` có `HTTP 404`, `RSS không hợp lệ`, `trang HTML thay vì RSS` | Feed đổi/gỡ | Bước 6, C.9 |
| `loi` có `context deadline exceeded`, `timeout`, `cannot connect to source` | Mạng hoặc nguồn chậm | Bước 6 |

### Bước 6. Kiểm tra RSS, trang gốc và CDN [CHỈ ĐỌC]

Kiểm tra trực tiếp nguồn có trả lời không (chạy trên máy đang chạy app, cùng mạng với app):
```sh
curl -sS -o /dev/null -w "%{http_code} %{content_type}\n" -A "PersonalNewsReader/0.1" https://vnexpress.net/rss/thoi-su.rss
```
(thay URL bằng URL feed đang lỗi, lấy từ thẻ nguồn).

| Kết quả | Nghĩa |
|---|---|
| `200 application/xml` hoặc `200 text/xml` | Feed bình thường. Nếu app vẫn báo lỗi → có thể lỗi ứng dụng (bước 7). |
| `404` | Feed không còn ở địa chỉ này (nguồn đổi/gỡ) |
| `429` / `503` | Nguồn đang giới hạn hoặc quá tải |
| `200 text/html` | Nguồn trả trang web thay vì RSS (đổi cấu trúc hoặc chặn) |
| `000` hoặc lỗi kết nối | Không kết nối được: mạng, DNS hoặc nguồn chặn |

Với bài cụ thể: mở **URL bài gốc** bằng trình duyệt (nút "Đọc bài gốc"). Nếu trang gốc cũng không có ảnh/video/nội dung, hoặc đã bị gỡ, thì app không thể hiển thị khác được.

Với ảnh: mở công cụ nhà phát triển của trình duyệt (F12 → tab Console) và tìm dòng đỏ có chữ `Content Security Policy` hoặc `404`. Xem C.13.

### Bước 7. Phân loại nguyên nhân

| Nhóm | Dấu hiệu | Ai xử lý |
|---|---|---|
| **Người dùng / thao tác** | Đang bật bộ lọc, sai địa chỉ, quên đăng nhập, cache trình duyệt | Bạn tự xử lý |
| **Cấu hình** | `.env` sai giá trị, `COOKIE_SECURE` không khớp HTTP/HTTPS, proxy không giữ header | Người vận hành (theo tài liệu) |
| **Hạ tầng** | Docker không chạy, hết ổ đĩa, mất mạng, VPS hết RAM | Người vận hành / nhà cung cấp VPS |
| **Ứng dụng (code/adapter)** | Feed trả 200 bình thường nhưng app vẫn lỗi; nhiều bài mới của một nguồn rơi vào "Chỉ có tóm tắt" do trang đổi cấu trúc; lỗi xuất hiện sau khi cập nhật code | Lập trình viên |
| **Giới hạn của nguồn** | Nguồn trả 429, chặn hotlink video, gỡ bài, RSS chậm, trang yêu cầu đăng nhập | Phụ thuộc website nguồn; không vượt qua cơ chế chặn |
| **Ngoài phạm vi hiện tại** | Muốn thêm website mới, phát video VnExpress/BBC trong app, dịch bài | Cần phát triển thêm (có thể làm được, nhưng là tính năng mới) |

---

## C. Danh mục lỗi thường gặp

### C.0. Bảng tra cứu nhanh

Nhóm xử lý: **[Tự xử lý]** = bạn làm theo hướng dẫn · **[Cấu hình/hạ tầng]** · **[Lập trình viên]** · **[Phụ thuộc nguồn]** · **[Ngoài phạm vi]**.

| # | Hiện tượng | Thường do | Nhóm |
|---|---|---|---|
| C.1 | Không mở được website | Sai địa chỉ/cổng, Docker hoặc container không chạy | Tự xử lý / Cấu hình |
| C.2 | Database không kết nối, hết dung lượng | db dừng, ổ đầy | Cấu hình/hạ tầng |
| C.3 | App không khởi động, log có `migration:` | Migration lỗi | Lập trình viên |
| C.4 | Không đăng nhập được, quên mật khẩu | Sai tên/mật khẩu, chưa tạo tài khoản | Tự xử lý |
| C.5 | "Đăng nhập sai quá nhiều lần…" | Giới hạn đăng nhập | Tự xử lý (chờ) |
| C.6 | Phiên hết hạn, "Yêu cầu không hợp lệ, hãy tải lại trang" | Hết hạn phiên, CSRF, cookie/HTTPS/proxy | Tự xử lý / Cấu hình |
| C.7 | Khách không thấy nút quản lý nguồn | Hành vi đúng | — |
| C.8 | Import báo lỗi | URL không hỗ trợ, URL bài viết, RSS 404, đã tồn tại | Tự xử lý / Phụ thuộc nguồn / Ngoài phạm vi |
| C.9 | Một feed báo lỗi liên tục | Feed đổi/gỡ, nguồn chặn | Phụ thuộc nguồn / Lập trình viên |
| C.10 | Không có bài mới | Feed chậm, nguồn tắt, worker lỗi/timeout/429 | Nhiều nhóm |
| C.11 | Bài chỉ có tóm tắt / đang chờ | Chưa đến lượt, trang video/tương tác, trang đổi cấu trúc | Chờ / Phụ thuộc nguồn / Lập trình viên |
| C.12 | Thiếu tác giả, ảnh, caption, nội dung | Nguồn không ghi, chưa bổ sung, cấu trúc lạ | Chờ / Phụ thuộc nguồn / Lập trình viên |
| C.13 | Ảnh lỗi | URL ảnh hỏng, CDN đổi, CSP, nguồn gỡ ảnh | Phụ thuộc nguồn / Lập trình viên |
| C.14 | Video không phát | Chỉ MP4 Tuổi Trẻ phát trong app; nguồn chặn hotlink/nhúng | Phụ thuộc nguồn / Ngoài phạm vi |
| C.15 | Bài trùng, đổi URL, bài gốc đã sửa nhưng dữ liệu cũ | Quy tắc chống trùng, không cập nhật lại toàn văn | Lập trình viên / Ngoài phạm vi |
| C.16 | Tìm kiếm, phân trang, lọc không như mong đợi | Cách lọc hoạt động, kết hợp bộ lọc | Tự xử lý |
| C.17 | Chấm đỏ không hiện / không mất / khác giữa thiết bị | Lưu theo từng trình duyệt | Tự xử lý |
| C.18 | Trang hiển thị cũ, giao diện mobile lỗi | Cache trình duyệt, màn hình hẹp | Tự xử lý / Lập trình viên |
| C.19 | Sau cập nhật app không chạy hoặc tính năng cũ hỏng | Migration, cấu hình mới, lỗi code | Lập trình viên |

---

### C.1. Không mở được website

**Triệu chứng:** trình duyệt báo "This site can't be reached", "Không thể kết nối", `ERR_CONNECTION_REFUSED`; hoặc trang trắng.

**Nguyên nhân có thể:** gõ sai địa chỉ/cổng; mở từ thiết bị khác (điện thoại, máy khác) trong khi app chỉ mở trên 127.0.0.1; Docker chưa chạy; container app dừng hoặc khởi động lại liên tục; đổi `APP_PORT` trong `.env`.

**Xác nhận:** [CHỈ ĐỌC]
```sh
docker compose ps
grep -E "^(APP_PORT|APP_BIND)=" .env
curl -s http://127.0.0.1:8080/healthz
```
- Cột PORTS cho biết địa chỉ thật, ví dụ `127.0.0.1:8080->8080/tcp` ⇒ mở `http://127.0.0.1:8080`. Nếu `APP_PORT=9090` thì mở `http://127.0.0.1:9090`.

**Xử lý (theo mức an toàn):**
1. [Tự xử lý] Dùng đúng địa chỉ trên đúng máy (A.12).
2. [THAY ĐỔI TRẠNG THÁI] Docker chưa chạy: mở Docker Desktop hoặc `colima start`, rồi `docker compose up -d`.
3. [THAY ĐỔI TRẠNG THÁI] Container dừng: `docker compose up -d`, đợi 30 giây, `docker compose ps`.
4. Container `Restarting`: `docker compose logs --tail 50 app`, tìm dòng lỗi cuối:
   - `migration: …` → C.3.
   - `POLL_INTERVAL must be >= 1m`, `COOKIE_SECURE must be true or false`, `ADMIN_SESSION_TTL must be a duration between …`, `ADMIN_SESSION_IDLE must be …`, `REPORT_EMAIL is not a valid address` → sửa giá trị tương ứng trong `.env` (ví dụ `2m`, `true`/`false`, `12h`), rồi `docker compose up -d app`.
   - `DATABASE_URL is invalid` → mật khẩu DB có ký tự đặc biệt; dùng chuỗi hex (A.3). **Lưu ý:** đổi `POSTGRES_PASSWORD` sau khi database đã tạo **không** đổi mật khẩu bên trong database; cần người hỗ trợ.
   - `merge duplicates: …` → C.15, nhờ lập trình viên.
5. Cổng 8080 bị chương trình khác chiếm (`port is already allocated` khi `up`): đặt `APP_PORT=8081` trong `.env`, `docker compose up -d app`, mở `http://127.0.0.1:8081`.

**Kiểm tra sau xử lý:** `docker compose ps` có `healthy`; healthz trả `ok`; trang mở được.

**Dừng và nhờ hỗ trợ khi:** log lỗi không có trong danh sách trên; app khởi động lại liên tục sau khi đã sửa `.env`.

---

### C.2. Database không kết nối được, hết dung lượng

**Triệu chứng:** healthz trả `database unavailable`; app `unhealthy`; danh sách tin báo "Không tải được danh sách tin"; log có `connection refused` hoặc `password authentication failed`.

**Nguyên nhân có thể:** container `db` dừng; ổ đĩa đầy (máy hoặc máy ảo Docker); mật khẩu trong `.env` khác mật khẩu đã dùng khi tạo database.

**Xác nhận:** [CHỈ ĐỌC]
```sh
docker compose ps db
docker compose exec db pg_isready -U news -d news
docker compose logs --tail 50 db
df -h
docker system df
```
- `docker system df` cho biết dung lượng image/container/volume Docker đang dùng.
- Trên macOS với Colima/Docker Desktop, ổ đĩa của máy ảo Docker có giới hạn riêng; `df -h` trên máy Mac có thể còn trống nhưng máy ảo đã đầy. Log db có `No space left on device` là dấu hiệu.

**Xử lý:**
1. [THAY ĐỔI TRẠNG THÁI] db dừng: `docker compose up -d db`, đợi `healthy`, rồi `docker compose restart app`.
2. [Cấu hình/hạ tầng] Hết dung lượng:
   - An toàn: xoá image cũ không dùng: `docker image prune` [THAY ĐỔI TRẠNG THÁI — không xoá volume]. Xoá file sao lưu cũ **sau khi đã chép ra nơi khác**.
   - **Không** dùng `docker system prune --volumes` hay `docker volume prune` để giải phóng chỗ (A.9).
   - Tăng ổ đĩa máy ảo Docker / VPS: làm cùng người hỗ trợ.
3. `password authentication failed`: mật khẩu `.env` đã bị đổi. Trả lại giá trị cũ nếu còn nhớ; nếu không, nhờ hỗ trợ (đổi mật khẩu bên trong PostgreSQL là thao tác [THAY ĐỔI DỮ LIỆU]).

**Kiểm tra sau xử lý:** `pg_isready` báo `accepting connections`; healthz `ok`; danh sách tin hiện lại.

**Dừng và nhờ hỗ trợ khi:** log db có `PANIC`, `corrupted`, `invalid page`; volume biến mất; không chắc lệnh giải phóng dung lượng có xoá dữ liệu không.

---

### C.3. Migration lỗi

**Triệu chứng:** app không khởi động, log có `migration: 0xx_ten.sql: …`.

**Nguyên nhân có thể:** dữ liệu hiện có vi phạm một ràng buộc mới; file migration bị sửa; database bị khôi phục không đầy đủ. Migration chạy trong một giao dịch: nếu lỗi, **không phần nào** của lần nâng cấp đó được ghi (dữ liệu giữ nguyên như trước khi khởi động).

**Xác nhận:** [CHỈ ĐỌC]
```sh
docker compose logs --tail 30 app | grep -i migration
docker compose exec db psql -U news news -c "SELECT name, applied_at FROM schema_migrations ORDER BY name;"
ls migrations/
```
So sánh: file nào có trong `migrations/` nhưng chưa có trong `schema_migrations` là file đang lỗi.

**Xử lý:**
1. [Lập trình viên] Không tự sửa file migration hay database. Gửi cho lập trình viên: dòng log lỗi, danh sách hai lệnh trên, commit đang dùng.
2. [THAY ĐỔI TRẠNG THÁI] Muốn app chạy lại ngay: quay về phiên bản code trước (D.6). Vì migration lỗi không ghi gì, code cũ thường chạy được trên database cũ.

**Kiểm tra sau xử lý:** log có `migration applied: …` cho file đó và `listening on :8080`.

**Dừng và nhờ hỗ trợ khi:** luôn luôn, với lỗi migration.

---

### C.4. Không đăng nhập được; quên mật khẩu

**Triệu chứng:** form đăng nhập báo **"Tên đăng nhập hoặc mật khẩu không đúng"**.

**Nguyên nhân có thể:** sai tên hoặc mật khẩu (thông báo cố ý giống nhau cho cả hai trường hợp); chưa tạo tài khoản; gõ nhầm chữ hoa/thường, bàn phím đang bật bộ gõ tiếng Việt.

**Xác nhận:** [CHỈ ĐỌC] `docker compose exec app /app/server admin status` cho biết tên đăng nhập (hoặc "Chưa có tài khoản quản trị").

**Xử lý:**
1. [Tự xử lý] Dùng đúng tên từ lệnh `status`; tắt bộ gõ tiếng Việt khi nhập mật khẩu.
2. [Tự xử lý, THAY ĐỔI DỮ LIỆU] Chưa có tài khoản → `admin create` (A.10).
3. [Tự xử lý, THAY ĐỔI DỮ LIỆU] Quên mật khẩu → `admin reset-password` (A.10). Mọi phiên cũ bị huỷ.

**Kiểm tra sau xử lý:** đăng nhập được, thấy "Nguồn tin".

**Dừng và nhờ hỗ trợ khi:** `status` báo lỗi kết nối database (C.2); đã reset mà vẫn không đăng nhập được.

---

### C.5. Bị giới hạn đăng nhập

**Triệu chứng:** **"Đăng nhập sai quá nhiều lần, hãy thử lại sau 15 phút"**.

**Nguyên nhân:** từ cùng một địa chỉ có **5 lần sai** trong 15 phút, hoặc toàn hệ thống có **30 lần sai** trong 15 phút. Khi bị khoá, kể cả mật khẩu đúng cũng bị từ chối cho đến khi hết thời gian.

**Xác nhận:** [CHỈ ĐỌC] `docker compose logs --since 30m app | grep "admin login failed"` — đếm số dòng.

**Xử lý:**
1. [Tự xử lý] Đợi 15 phút rồi thử lại với mật khẩu đúng (dùng `admin status` để chắc tên đăng nhập).
2. [THAY ĐỔI TRẠNG THÁI] Bộ đếm nằm trong bộ nhớ app, nên `docker compose restart app` xoá bộ đếm. Chỉ làm khi chắc chắn **chính bạn** là người gõ sai.
3. Nếu thấy nhiều dòng `admin login failed` từ địa chỉ lạ (khi đã mở ra Internet): có người dò mật khẩu. Đổi sang mật khẩu dài hơn (`reset-password`), xem xét giới hạn ở proxy/tường lửa. [Cấu hình/hạ tầng]

**Kiểm tra sau xử lý:** đăng nhập được.

---

### C.6. Phiên hết hạn; lỗi CSRF; cookie, HTTPS và reverse proxy

**Triệu chứng và nghĩa:**

| Thấy | Nghĩa |
|---|---|
| Đang ở "Nguồn tin" bị chuyển về form đăng nhập với "Phiên đăng nhập đã hết hạn. Hãy đăng nhập lại." | Phiên quá 12 giờ, hoặc 2 giờ không thao tác, hoặc đã đăng xuất/reset mật khẩu ở nơi khác |
| Thao tác báo **"Cần đăng nhập quản trị"** | Như trên, hoặc trình duyệt không gửi cookie |
| Thao tác báo **"Yêu cầu không hợp lệ, hãy tải lại trang"** | Kiểm tra chống giả mạo yêu cầu (CSRF) thất bại |
| Đăng nhập báo thành công nhưng thao tác ngay sau đó bị đòi đăng nhập lại | Cookie không được lưu/gửi (thường do `COOKIE_SECURE` không khớp) |

**CSRF là gì:** một trang web lạ có thể lợi dụng việc bạn đang đăng nhập để gửi lệnh thay bạn. App chặn bằng cách yêu cầu mỗi lệnh ghi phải kèm một mã bí mật của phiên (chỉ trang của app biết) và phải đến từ đúng trang của app.

**Nguyên nhân có thể:** trang mở quá lâu (mã CSRF trong trang cũ); dùng hai tab, đăng nhập lại ở tab kia; `COOKIE_SECURE=true` khi đang dùng `http://` (trình duyệt không lưu cookie Secure qua HTTP); reverse proxy đổi `Host` hoặc `Origin`.

**Xác nhận:**
- [CHỈ ĐỌC] `grep "^COOKIE_SECURE=" .env` — phải là `false` khi dùng `http://127.0.0.1`, `true` khi dùng `https://`.
- F12 → tab Application (Chrome) → Cookies → địa chỉ trang: có cookie `nr_admin` (HTTP) hoặc `__Host-nr_admin` (HTTPS) sau khi đăng nhập.

**Xử lý:**
1. [Tự xử lý] Tải lại trang (F5), đăng nhập lại.
2. [Cấu hình] Sửa `COOKIE_SECURE` cho khớp, `docker compose up -d app`, đăng nhập lại.
3. [Cấu hình/hạ tầng] Sau reverse proxy: proxy phải chuyển nguyên `Host` và `Origin`. (Chưa có cấu hình proxy được kiểm thử trong repo.)
4. [Tự xử lý] Muốn phiên dài hơn: `ADMIN_SESSION_TTL` (tối đa `720h`), `ADMIN_SESSION_IDLE` (không lớn hơn TTL), rồi `docker compose up -d app`.

**Kiểm tra sau xử lý:** đổi tên một nguồn rồi đổi lại thành công.

**Dừng và nhờ hỗ trợ khi:** lỗi chỉ xảy ra sau proxy/HTTPS và bước 3 không giải quyết.

---

### C.7. Khách xem được tin nhưng không quản lý được nguồn

**Đây là hành vi đúng.** Thiết kế hiện tại:
- Khách (không đăng nhập): đọc danh sách, đọc bài, tìm kiếm, lọc theo chuyên mục/quốc gia/nguồn.
- Chỉ quản trị viên: thêm/import, đổi tên, đổi chuyên mục, chọn quốc gia, bật/tắt, xoá nguồn, xem trạng thái lấy tin và lỗi chi tiết.

Nút "Nguồn tin" bị ẩn với khách, và **máy chủ cũng từ chối** mọi thao tác quản trị nếu không có phiên hợp lệ (trả lỗi 401). Gọi trực tiếp địa chỉ API quản trị cũng không qua được.

**Chỉ là lỗi khi:** quản trị viên **đã đăng nhập** mà vẫn không thấy "Nguồn tin" → xem C.6.

---

### C.8. Import (thêm nguồn) báo lỗi

Nhận diện URL được làm **chỉ từ chữ trong URL**, chưa gửi request nào. Chỉ khi URL được nhận diện, app mới đọc thử RSS một lần.

| Thông báo | Nghĩa | Nhóm | Làm gì |
|---|---|---|---|
| "URL không hợp lệ. Hãy dán URL đầy đủ…" | Không phải URL, có dấu cách, quá dài | Tự xử lý | Dán lại URL đầy đủ bắt đầu bằng `https://` |
| "Chỉ hỗ trợ URL https." | URL `http://` | Tự xử lý | Đổi thành `https://` |
| "URL không được chỉ định cổng khác 443." / "…tên đăng nhập hoặc mật khẩu." | URL lạ | Tự xử lý | Dùng URL công khai thông thường |
| "Nguồn này chưa được hỗ trợ. Hiện hỗ trợ VnExpress, BBC News và Tuổi Trẻ." | Website khác | **Ngoài phạm vi** | Cần lập trình viên viết adapter mới (xem tài liệu giải thích, mục G) |
| "Đây là URL của một bài viết, chưa phải URL nguồn tin…" | Dán link bài | Tự xử lý | Dán trang chủ, trang chuyên mục có ánh xạ, hoặc RSS |
| Trang chuyên mục bị từ chối (ví dụ trang chuyên mục VnExpress không phải RSS) | Chuyên mục đó chưa có ánh xạ sang RSS | Tự xử lý / Lập trình viên | Dùng URL RSS của chuyên mục. VnExpress: danh sách tại https://vnexpress.net/rss ; Tuổi Trẻ: https://tuoitre.vn/rss.htm |
| "Nguồn này đã có trong danh sách." | Feed đã có (không phải lỗi) | — | Không cần làm gì; feed đang tắt sẽ được bật lại ("đã bật lại") |
| "Không thêm được nguồn: không đọc được RSS: nguồn trả HTTP 404." | Feed không tồn tại ở URL đó | Phụ thuộc nguồn / Tự xử lý | Kiểm tra lại URL trên trang danh sách RSS của báo |
| "… RSS không hợp lệ …", "… trang HTML thay vì RSS XML", "RSS không có bài" | URL không phải RSS hoặc nguồn đổi định dạng | Phụ thuộc nguồn | Kiểm tra bằng bước 6 |
| "… nguồn chuyển hướng tới …, ngoài phạm vi được phép" | Nguồn chuyển hướng sang địa chỉ lạ | Phụ thuộc nguồn / Lập trình viên | Báo lập trình viên nếu đó là địa chỉ chính thức mới của báo |
| "… nguồn đang giới hạn tần suất truy cập: HTTP 429" | Nguồn yêu cầu chậm lại | Phụ thuộc nguồn | Thử lại sau |
| "Đã thêm X kênh, Y kênh lỗi." | Import trang chủ: một số feed lỗi | Phụ thuộc nguồn | Mở "Chi tiết" xem feed nào lỗi |
| "Đang có một lượt import khác, hãy đợi nó xong." / "Import quá nhiều lần trong một phút…" | Giới hạn: 1 lượt cùng lúc, 6 lượt/phút | Tự xử lý | Đợi 1 phút |
| "Phiên đăng nhập đã hết…" | Hết phiên | Tự xử lý | Đăng nhập lại (C.6) |

Nút **"Báo lỗi qua email"** xuất hiện với lỗi import; nó mở ứng dụng email với **chỉ URL bạn đã nhập** (không kèm thông tin khác). Địa chỉ nhận đặt bằng biến `REPORT_EMAIL` (để trống thì dùng địa chỉ mặc định trong code).

**Kiểm tra sau xử lý:** thẻ nguồn mới xuất hiện; vài phút sau "Kiểm tra gần nhất" có giờ và bài mới xuất hiện ở danh sách.

---

### C.9. Một feed báo lỗi liên tục

**Triệu chứng:** thẻ nguồn có dòng đỏ, "Thành công" dừng ở một thời điểm cũ.

**Xác nhận:** bước 5 và bước 6 của mục B.

| Lỗi trên thẻ | Nguyên nhân | Nhóm | Xử lý |
|---|---|---|---|
| `nguồn trả HTTP 404` | Báo đã gỡ/đổi địa chỉ feed | Phụ thuộc nguồn | Tìm URL RSS mới trên trang RSS của báo, import URL mới, xoá feed cũ |
| `nguồn trả HTTP 403` / `401` | Nguồn chặn truy cập | Phụ thuộc nguồn | Không vượt chặn. Tắt feed nếu kéo dài |
| `nguồn đang giới hạn tần suất truy cập: HTTP 429/503` | Nguồn yêu cầu chậm lại | Phụ thuộc nguồn | Tự hết; worker đã tự dừng site đó trong lượt. Nếu kéo dài nhiều giờ: tăng `POLL_INTERVAL` (ví dụ `5m`) [Cấu hình] |
| `RSS không hợp lệ: …` / `trang HTML thay vì RSS XML` | Feed hỏng hoặc trả trang khác | Phụ thuộc nguồn / Lập trình viên | Kiểm tra bằng curl (bước 6). Nếu báo đổi định dạng hợp lệ, cần lập trình viên |
| `context deadline exceeded`, `cannot connect to source`, `i/o timeout` | Mạng chậm/mất, nguồn chậm | Hạ tầng / Phụ thuộc nguồn | Kiểm tra Internet; thường tự hết |
| `blocked non-public address` | Tên miền nguồn trỏ tới địa chỉ nội bộ (cơ chế an toàn chặn) | Hạ tầng / Lập trình viên | Kiểm tra DNS của máy chủ; báo lập trình viên |
| `nguồn chuyển hướng tới …, ngoài phạm vi được phép` | Nguồn chuyển hướng ra ngoài danh sách cho phép | Lập trình viên | Báo kèm thông báo đầy đủ |

---

### C.10. Không có bài mới

**Triệu chứng:** danh sách không có bài mới trong nhiều giờ, hoặc thiếu bài bạn thấy trên trang chủ báo.

**Nguyên nhân có thể (từ thường gặp đến hiếm):**
1. **RSS của nguồn chậm hoặc không đủ:** RSS là danh sách báo tự cung cấp; không phải mọi bài trên trang chủ đều có trong RSS, và có feed cập nhật chậm. Ví dụ đo được ngày 2026-10-05 (chưa đo lại): feed "tin mới nhất" của VnExpress chậm khoảng 3 giờ so với trang chủ, trong khi feed chuyên mục mới hơn. [Phụ thuộc nguồn]
2. Đang bật bộ lọc (quốc gia/nguồn/từ khoá/chuyên mục). [Tự xử lý]
3. Bài thuộc chuyên mục mà không feed nào của chuyên mục đó liệt kê. [Tự xử lý: thêm feed chuyên mục đó]
4. Nguồn đang **tắt** hoặc đã **xoá**. [Tự xử lý]
5. Nguồn trả lỗi 429/503/404 (C.9). [Phụ thuộc nguồn]
6. Worker không chạy (app dừng, bị kẹt, mất mạng). [Cấu hình/hạ tầng]

**Xác nhận:** bước 2 → bước 5 → bước 6 của mục B. Kiểm tra lần nhận bài gần nhất:
```sh
docker compose exec db psql -U news news -c "SELECT max(fetched_at) FROM articles;"
```
(`fetched_at` là lúc hệ thống nhận bài, giờ UTC — chậm hơn giờ Việt Nam 7 tiếng.)

**Xử lý:**
1. Đặt mọi bộ lọc về "Tất cả", bấm "Tải lại".
2. Bật nguồn đang tắt (màn hình Nguồn tin).
3. [THAY ĐỔI TRẠNG THÁI] Nếu mọi nguồn có `last_checked` cũ hàng giờ: `docker compose restart app` (worker chạy ngay khi khởi động), đợi 2–3 phút, kiểm tra lại.
4. Thiếu bài từ một chuyên mục cụ thể: import feed RSS của chuyên mục đó (nếu nằm trong 3 website được hỗ trợ).

**Kiểm tra sau xử lý:** `max(fetched_at)` gần thời điểm hiện tại; `last_checked` của các nguồn mới cập nhật.

**Không cam kết:** chu kỳ 2 phút chỉ là **lịch kiểm tra**. Bài xuất hiện khi **nguồn đã đưa bài vào RSS**; nếu nguồn chậm thì app cũng chậm theo.

**Dừng và nhờ hỗ trợ khi:** worker không cập nhật `last_checked` dù app đã restart và mạng bình thường.

---

### C.11. Bài chỉ có tóm tắt hoặc đang chờ toàn văn

**Các nhãn trên thẻ bài:**

| Nhãn | Nghĩa |
|---|---|
| **Toàn văn** | Đã lấy đầy đủ nội dung bài từ trang gốc |
| **Tóm tắt · đang chờ lấy nội dung** | Mới có phần tóm tắt từ RSS, trang bài chưa được đọc |
| **Chỉ có tóm tắt** | Đã thử nhưng không lấy được toàn văn (hoặc trang không có toàn văn dạng chữ) |

**Nguyên nhân "đang chờ":** mỗi lượt worker chỉ đọc tối đa **12 trang bài mỗi website**, cách nhau 1,5 giây, trong tối đa 60 giây — để không gây tải cho báo. Khi có nhiều bài mới (ví dụ vừa thêm nguồn), phần tồn đọng được lấy dần. Bài mới nhất được ưu tiên.

**Nguyên nhân "chỉ có tóm tắt":**
- Trang video, ảnh, trò chơi/tương tác (crossword, quiz…), tường thuật trực tiếp: không có đủ chữ (dưới khoảng 300 ký tự được coi là không đủ). [Phụ thuộc nguồn — hành vi đúng]
- Bài đã bị gỡ (trang lỗi). [Phụ thuộc nguồn]
- Trang đổi cấu trúc nên app không tìm thấy vùng nội dung. [Lập trình viên]
- Lỗi mạng 3 lần liên tiếp (thử lại sau 30 phút, rồi 60 phút; sau 3 lần thì giữ tóm tắt). [Hạ tầng/nguồn]

**Xác nhận:** [CHỈ ĐỌC] lấy lý do của một bài (thay phần trong ngoặc bằng một đoạn của URL bài gốc):
```sh
docker compose exec db psql -U news news -c "SELECT id, content_status, attempts, next_attempt, left(content_error,150) FROM articles WHERE url LIKE '%doan-url-bai%';"
```
| `content_error` | Nghĩa |
|---|---|
| `bài dạng tương tác (…)` | Trang tương tác — đúng là không có toàn văn |
| `nội dung không đủ hoặc cấu trúc trang đã thay đổi` | Trang ít chữ (video/ảnh) hoặc cấu trúc đổi |
| `chưa tìm thấy vùng nội dung bài` | Cấu trúc trang khác thường |
| `nguồn trả HTTP 404` | Bài đã gỡ |
| `lỗi xử lý cấu trúc trang: …` | Trang có cấu trúc gây lỗi cho bộ trích xuất (đã được chặn để không làm sập worker) |

Theo dõi tổng thể: [CHỈ ĐỌC]
```sh
docker compose exec db psql -U news news -c "SELECT s.adapter, a.content_status, count(*) FROM articles a JOIN sources s ON s.id=a.source_id GROUP BY 1,2 ORDER BY 1,2;"
```

**Xử lý:**
- "Đang chờ": đợi. Không cần thao tác.
- "Chỉ có tóm tắt" vì video/tương tác/bài gỡ: hành vi đúng; đọc tại nguồn qua nút "Đọc bài gốc".
- **Nhiều bài mới của cùng một nguồn** đột nhiên rơi vào "Chỉ có tóm tắt" với lỗi cấu trúc: dấu hiệu báo đổi giao diện → [Lập trình viên] sửa bộ trích xuất của nguồn đó.

**Dừng và nhờ hỗ trợ khi:** tỷ lệ "Chỉ có tóm tắt" của một nguồn tăng rõ rệt trong một ngày.

---

### C.12. Thiếu tác giả, ảnh, caption hoặc nội dung

**Nguyên nhân có thể:**
- **Nguồn không ghi:** nhiều bài không có dòng tác giả. VnExpress chỉ có tác giả khi cuối bài có dòng ký tên; app không đoán. Ảnh không có chú thích thì để trống. [Phụ thuộc nguồn — hành vi đúng]
- **Chưa bổ sung:** bài lưu từ phiên bản cũ của bộ trích xuất được đọc lại dần (tối đa 6 bài/website/lượt, dùng phần thời gian còn lại sau bài mới). Trang bài hiện dòng "Ảnh và tác giả của bài này đang chờ được bổ sung từ trang nguồn." [Chờ]
- **Loại có chủ ý:** app bỏ quảng cáo, thumbnail bài liên quan, ảnh đại diện tác giả, banner đăng ký bản tin, ảnh nằm ngoài danh sách máy chủ ảnh được phép. [Hành vi đúng]
- **Cấu trúc lạ:** bảng, đồ hoạ tương tác, nội dung dựng bằng JavaScript không được lấy. [Ngoài phạm vi hiện tại]
- **Nguồn sửa bài sau khi đã lấy:** app không lấy lại toàn văn (C.15).

**Xác nhận:** mở bài gốc so sánh. Nếu bài gốc có tác giả/ảnh mà app không có, và bài đã ở trạng thái "Toàn văn" không còn dòng "đang chờ bổ sung" → ghi lại URL, báo lập trình viên.

Tiến độ bổ sung: màn hình Nguồn tin (dòng thông báo xanh) hoặc [CHỈ ĐỌC]:
```sh
docker compose exec db psql -U news news -c "SELECT count(*) FILTER (WHERE enrich_error<>'') AS loi_bo_sung FROM articles;"
```

**Xử lý:** chờ hoặc báo lập trình viên với URL cụ thể. Không có thao tác nào cho người dùng để "lấy lại" một bài.

---

### C.13. Ảnh lỗi

**Triệu chứng:** khung ảnh hiện **"Không tải được ảnh từ nguồn."**; hoặc không có ảnh dù bài gốc có.

**Cách app hiển thị ảnh:** app **không lưu file ảnh**. Trình duyệt của bạn tải ảnh **trực tiếp** từ máy chủ ảnh (CDN) của báo. Chỉ các máy chủ sau được phép:
- BBC: `ichef.bbci.co.uk`
- VnExpress: các máy chủ kết thúc bằng `.vnecdn.net`
- Tuổi Trẻ: `cdn2.tuoitre.vn`

Danh sách này được kiểm ở hai nơi: khi lưu bài (máy chủ) và bằng chính sách bảo mật nội dung **CSP** của trang (trình duyệt từ chối tải ảnh từ máy chủ khác).

**Nguyên nhân có thể:** báo gỡ ảnh hoặc đổi địa chỉ; CDN của báo lỗi tạm thời; báo chuyển sang máy chủ ảnh mới (CSP chặn); mạng của bạn chặn CDN đó; tiện ích chặn quảng cáo.

**Xác nhận:**
1. Mở bài gốc: ảnh có hiện không?
2. F12 → Console: dòng `Refused to load the image … Content Security Policy` ⇒ máy chủ ảnh mới chưa có trong danh sách. Dòng `404` ⇒ ảnh bị gỡ.
3. Thử tắt tiện ích chặn quảng cáo / dùng cửa sổ ẩn danh.

**Xử lý:**
- Ảnh bị gỡ / CDN lỗi tạm: [Phụ thuộc nguồn] — không xử lý được phía app.
- Báo đổi máy chủ ảnh: [Lập trình viên] cập nhật danh sách máy chủ ảnh ở **cả hai nơi** (code kiểm tra URL ảnh và CSP), có kiểm thử.
- Tiện ích trình duyệt: [Tự xử lý] cho phép trang này.

---

### C.14. Video không phát

**Hành vi hiện tại theo nguồn (đúng theo code):**

| Nguồn | Trong app | Lý do |
|---|---|---|
| Tuổi Trẻ — trang `/video/` | Có nút **"Xem video"**, phát trực tiếp từ `cdn2.tuoitre.vn` (MP4) | File MP4 công khai, đã kiểm tra phát được |
| Tuổi Trẻ — video chèn trong bài thường | Chỉ ảnh + **"Xem video tại nguồn"** | Địa chỉ file chưa được kiểm chứng |
| VnExpress | Chỉ ảnh + "Xem video tại nguồn" | Nguồn **chặn hotlink**: chỉ cho phát khi đang ở trang vnexpress.net |
| BBC | Chỉ ảnh + "Xem video tại nguồn" | Trình phát nhúng chính thức không hiển thị khi nhúng vào trang khác |

Nhãn "Video" trên thẻ bài chỉ có nghĩa là **bài có video**, không có nghĩa phát được trong app.

**Thông báo khi phát MP4 Tuổi Trẻ lỗi:**

| Thông báo | Nghĩa |
|---|---|
| "Không phát được video từ nguồn (video có thể đã bị gỡ hoặc bị chặn)." | File bị gỡ/chặn |
| "Trình duyệt này không phát được định dạng video của nguồn. Hãy xem tại nguồn." | Trình duyệt không hỗ trợ định dạng |
| "Video tải quá lâu nên đã dừng. Hãy thử lại hoặc xem tại nguồn." | Mạng chậm |

**Xử lý:** dùng nút "Xem video tại nguồn". **Không** giả mạo Referer hay dùng proxy để vượt cơ chế chặn của nguồn. Phát video của nguồn khác trong app là [Ngoài phạm vi] và cần nguồn cho phép (ví dụ trình phát nhúng chính thức hoạt động), sau đó mới phát triển thêm.

---

### C.15. Bài trùng, bài đổi URL, bài gốc đã sửa

**Cơ chế chống trùng hiện có:**
- URL được **chuẩn hoá** trước khi lưu: bỏ phần theo dõi quảng cáo (`utm_…`, `at_…`, `fbclid`, `gclid`, `ocid`), bỏ phần `#…`, chữ thường cho tên máy chủ; BBC gom `bbc.com` và `bbc.co.uk` về một. Mỗi URL chuẩn hoá chỉ lưu **một lần** (ràng buộc duy nhất trong database).
- VnExpress (`…-<mã số>.html`) và Tuổi Trẻ (`…-<mã số>.htm`) còn được nhận diện bằng **mã bài**: báo đổi tiêu đề/đường dẫn thì vẫn là một bài; link và tiêu đề đi theo đường dẫn mới.
- Một bài xuất hiện trong nhiều feed vẫn chỉ là **một bài**, thuộc chuyên mục của mọi feed đã liệt kê nó.
- Khi khởi động, app gộp các bản trùng cũ có cùng mã bài (log `merge duplicates: …`). Hai bản có thời gian đăng khác nhau thì **không** gộp tự động, ghi "kept apart" trong log.

**Khi nào vẫn thấy trùng:**
- BBC không có mã bài ổn định trong URL: nếu BBC đổi hẳn URL của bài, có thể thành hai bài. [Giới hạn hiện tại / Lập trình viên]
- Hai bài khác nhau nhưng tiêu đề giống nhau (tin cập nhật). [Hành vi đúng]
- Log "kept apart": cần người xem tay. [Lập trình viên]

**Bài gốc đã sửa:** app **không** lấy lại toàn văn khi báo sửa bài sau khi đã lấy (tiêu đề VnExpress/Tuổi Trẻ có thể đổi theo URL mới nếu feed đưa URL mới). [Ngoài phạm vi hiện tại — có thể phát triển thêm]

**Xác nhận trùng:** [CHỈ ĐỌC]
```sh
docker compose logs app | grep "merge duplicates"
docker compose exec db psql -U news news -c "SELECT id, url, title, published_at FROM articles WHERE title ILIKE '%mot-phan-tieu-de%' ORDER BY id;"
```

**Xử lý:** gửi hai URL/ID bị trùng cho lập trình viên. Không tự xoá dòng trong database.

---

### C.16. Tìm kiếm, phân trang, lọc chuyên mục/quốc gia/nguồn

| Hiện tượng | Giải thích | Làm gì |
|---|---|---|
| Tìm không ra bài có từ đó trong nội dung | Tìm kiếm **chỉ theo tiêu đề** | Dùng từ có trong tiêu đề |
| Tìm "bao" không ra "bão" | So khớp đúng chữ, có phân biệt dấu (không phân biệt hoa/thường) | Gõ đúng dấu |
| Gõ `%` hoặc `_` | Được hiểu là ký tự thường, không phải ký tự đại diện | — |
| Từ khoá quá dài | Tối đa 200 ký tự | Rút gọn |
| "Không có bài phù hợp bộ lọc." | Kết hợp bộ lọc không có bài nào (ví dụ Vương quốc Anh + chuyên mục Thời sự: BBC hiện không có feed Thời sự) | Bỏ bớt bộ lọc |
| "Chuyên mục này chưa có bài…" | Chưa có feed nào được gán vào chuyên mục | Quản trị viên gán feed vào chuyên mục |
| Chọn quốc gia thì ô Nguồn trở về "Tất cả nguồn" | Nguồn đang chọn không thuộc quốc gia mới | Hành vi đúng |
| Không thấy "Thái Lan" trong bộ lọc | Bộ lọc chỉ hiện nước **đang có nguồn**. Hiện chưa có nguồn Thái Lan | Hành vi đúng; danh sách đầy đủ chỉ có khi quản trị chọn quốc gia |
| Bài BBC viết về Việt Nam nằm ở "Vương quốc Anh" | Quốc gia là của **tờ báo**, không phải nội dung | Hành vi đúng |
| "Không còn bài ở trang này." | Đã hết bài | Bấm "Trang trước" |
| Bài nhảy trang khi đang xem | Có bài mới chen vào đầu danh sách | Bình thường với danh sách theo thời gian |

---

### C.17. Chấm đỏ (bài mới) không xuất hiện, không mất, hoặc khác giữa thiết bị

**Cách hoạt động:** chấm đỏ trên chuyên mục = có bài **hệ thống nhận được** sau lần cuối **bạn mở chuyên mục đó trên trình duyệt này**. Trạng thái lưu trong bộ nhớ của trình duyệt (localStorage, khoá `news-seen-v1`), **không** lưu trên máy chủ.

| Hiện tượng | Lý do | Làm gì |
|---|---|---|
| Lần đầu mở không có chấm nào | Lần đầu, mọi bài hiện có được coi là đã xem (để không đánh dấu cả lịch sử là mới) | Bình thường |
| Chấm không mất khi đang lọc quốc gia/nguồn/từ khoá | Bạn mới xem **một phần** chuyên mục, nên không đánh dấu đã xem | Bỏ bộ lọc rồi mở lại chuyên mục |
| Chấm không mất khi ở "Tất cả" | Màn hình Tất cả không đánh dấu chuyên mục nào | Mở từng chuyên mục |
| Tự làm mới 2 phút không xoá chấm | Cố ý: chỉ khi **bạn** mở chuyên mục mới đánh dấu | — |
| Máy khác/trình duyệt khác/tab ẩn danh thấy khác | Mỗi trình duyệt lưu riêng | Bình thường; chưa đồng bộ giữa thiết bị |
| `127.0.0.1:8080` và `localhost:8080` khác nhau | Trình duyệt coi là hai trang khác nhau | Dùng cố định một địa chỉ |
| Xoá dữ liệu trình duyệt → mất trạng thái | Trạng thái nằm trong trình duyệt | Sẽ bắt đầu lại như lần đầu |
| Bài đăng từ hôm qua vẫn có chấm | Chấm theo thời điểm **hệ thống nhận** bài, không theo giờ đăng | Bình thường |

---

### C.18. Trang hiển thị cũ do cache; giao diện mobile lỗi

**Cache:** máy chủ yêu cầu trình duyệt kiểm tra lại file giao diện mỗi lần tải (`Cache-Control: no-cache`) và không lưu dữ liệu API (`no-store`). Sau cập nhật, nếu giao diện vẫn cũ hoặc nút bấm không phản hồi:
1. [Tự xử lý] Tải lại cứng: Cmd+Shift+R (Mac) / Ctrl+Shift+R (Windows).
2. Thử cửa sổ ẩn danh.
3. F12 → Console: chép dòng lỗi đỏ gửi lập trình viên.

**Mobile:** giao diện đã được kiểm ở bề rộng 375 px **bằng giả lập** (Chrome headless), **chưa thử trên điện thoại thật**. Nếu thấy tràn ngang, chữ bị cắt, nút không bấm được: chụp màn hình, ghi tên máy, hệ điều hành, trình duyệt, gửi lập trình viên. [Lập trình viên]

---

### C.19. Sau cập nhật app không chạy hoặc tính năng cũ bị lỗi

**Nguyên nhân có thể:** migration mới lỗi (C.3); bản mới cần biến cấu hình mới hoặc đổi tên biến; lỗi code; trình duyệt dùng file cũ (C.18).

**Xác nhận:** [CHỈ ĐỌC]
```sh
git log -1 --oneline
docker compose ps
docker compose logs --tail 50 app
```
So sánh `.env` với `.env.example` mới: có biến mới nào không.

**Xử lý:**
1. Lỗi cấu hình (log báo `… must …`): sửa `.env` theo `.env.example`, `docker compose up -d app`. [Cấu hình]
2. Lỗi migration: C.3.
3. Lỗi tính năng: quay về bản trước (D.6) và báo lập trình viên với mẫu ở mục E.

**Ví dụ có thật trong lịch sử project:** bản cập nhật ngày 2026-10-06 **bỏ** đăng nhập bằng mã `ADMIN_TOKEN` và chế độ `LOCAL_NO_AUTH`; thay bằng đọc công khai + tài khoản quản trị. Sau cập nhật này phải **tạo tài khoản quản trị** (A.10) mới vào được "Nguồn tin". Các báo cáo cũ (AUDIT_REPORT.md, phần đầu CHANGE_REPORT.md) còn nhắc tới mã truy cập — đó là mô tả phiên bản cũ.

---

## D. Sao lưu, cập nhật và khôi phục

### D.1. Cơ chế sao lưu hiện có

- **Chỉ có sao lưu thủ công** bằng lệnh `pg_dump` (D.2). **Chưa có** sao lưu tự động theo lịch, **chưa có** script sao lưu/khôi phục trong repo.
- Thư mục `backups/` trong project chứa các bản sao lưu đã tạo trong quá trình phát triển. Thư mục này **không** được đưa vào Git và **không** được đưa vào image Docker.
- Sao lưu chỉ gồm **database** (nguồn, bài, chuyên mục, quốc gia, tài khoản quản trị dạng hash, phiên). Ảnh/video không nằm trong database (chỉ có địa chỉ), nên không cần sao lưu file media.
- File `.env` **không** nằm trong bản sao lưu database. Cất riêng một bản ở nơi an toàn (trình quản lý mật khẩu), vì cần mật khẩu DB để chạy lại.

### D.2. Tạo bản sao lưu

**Làm ở đâu:** máy chạy Docker (local hoặc VPS), thư mục project. **Điều kiện:** container `db` đang chạy. **Mức:** [CHỈ ĐỌC] với database (chỉ đọc ra file). **Xác minh:** Đã dùng trong project.

```sh
mkdir -p backups
docker compose exec -T db pg_dump -U news -Fc news > backups/news-$(date +%Y%m%d-%H%M).dump
ls -la backups/ | tail -3
```

**Kết quả mong đợi:** một file `news-YYYYMMDD-HHMM.dump`, dung lượng vài MB trở lên (tuỳ số bài). App vẫn chạy bình thường trong lúc sao lưu.
**Nếu khác:** file 0 byte hoặc rất nhỏ → lệnh lỗi; xem thông báo trên màn hình, kiểm tra db đang chạy (C.2). Phải có `-T` sau `exec`, nếu không file có thể bị hỏng.

### D.3. Kiểm tra file sao lưu

**Mức:** [CHỈ ĐỌC]. **Xác minh:** Đã dùng trong project.

```sh
docker run --rm -i postgres:17-alpine pg_restore --list < backups/TEN-FILE.dump | grep -c "TABLE DATA"
```
**Kết quả mong đợi:** một con số bằng số bảng có dữ liệu (với phiên bản hiện tại khoảng 9 bảng; số chính xác tuỳ phiên bản). Lệnh không báo lỗi.
**Nếu khác:** `pg_restore: error: input file does not appear to be a valid archive` → file hỏng, tạo lại bản sao lưu.

Kiểm tra `--list` chỉ cho biết file đọc được. Cách kiểm tra chắc chắn là **khôi phục thử vào một stack riêng** (D.5).

### D.4. Lưu bản sao ngoài máy/VPS

Bản sao lưu nằm cùng máy với database sẽ mất cùng lúc nếu máy hỏng. Cần ít nhất một bản ở nơi khác.

- Local: chép file `.dump` sang ổ ngoài hoặc dịch vụ lưu trữ đám mây.
- VPS (Chưa kiểm chứng): từ **máy cá nhân**, tải về:
  ```sh
  scp <user>@<IP-VPS>:<thu-muc-project>/backups/TEN-FILE.dump ./
  ```
- File sao lưu chứa toàn bộ dữ liệu, gồm hash mật khẩu quản trị: lưu ở nơi có kiểm soát truy cập, không chia sẻ công khai, không gửi cho AI.

**Khuyến nghị (chưa được thiết lập):** sao lưu hằng ngày, giữ 7 bản gần nhất và 1 bản mỗi tuần, thử khôi phục mỗi tháng.

### D.5. Khôi phục thử vào stack riêng (không đụng database thật)

**Mục đích:** chứng minh bản sao lưu dùng được, hoặc xem lại dữ liệu cũ, **mà không ghi đè** database đang chạy.
**Xác minh:** Cách làm dưới đây đã được dùng một lần trong project (2026-10-06) để kiểm thử bản cập nhật; **không phải script có sẵn**, gõ lại từng lệnh.
**Mức:** [THAY ĐỔI TRẠNG THÁI] — tạo container và mạng Docker tạm, không đụng `news-reader_pgdata`.
**Điều kiện:** image app đã được build (`docker compose build app` hoặc đã chạy `up --build` trước đó); cổng 8081 còn trống.

```sh
# 1. Mạng và database tạm (mật khẩu tạm chỉ dùng cho bản thử)
docker network create nr-restore-test
docker run -d --name nr-restore-db --network nr-restore-test -e POSTGRES_PASSWORD=tam-thoi-chi-de-thu -e POSTGRES_DB=thu postgres:17-alpine
# đợi khoảng 10 giây, rồi kiểm tra:
docker exec nr-restore-db pg_isready -U postgres -d thu

# 2. Khôi phục bản sao lưu vào database tạm
docker exec -i nr-restore-db pg_restore -U postgres -d thu --no-owner < backups/TEN-FILE.dump
docker exec nr-restore-db psql -U postgres -d thu -c "SELECT count(*) FROM articles;"

# 3. (Tuỳ chọn) chạy app trên bản khôi phục, cổng 8081
docker run -d --name nr-restore-app --network nr-restore-test -p 127.0.0.1:8081:8080 \
  -e DATABASE_URL='postgres://postgres:tam-thoi-chi-de-thu@nr-restore-db:5432/thu?sslmode=disable' \
  -e LISTEN_ADDR=:8080 -e POLL_INTERVAL=24h -e FULL_TEXT_ENABLED=false \
  news-reader-app:latest
# mở http://127.0.0.1:8081 để xem
```
Ghi chú: tên image `news-reader-app` phụ thuộc tên thư mục project; xem bằng `docker images | grep news`. App thử vẫn lấy tin một lượt khi khởi động (ghi vào database **tạm**, không ảnh hưởng database thật).

**Kết quả mong đợi:** bước 2 không báo lỗi (có thể có cảnh báo về quyền sở hữu, bỏ qua được khi dùng `--no-owner`); số bài hợp lý; trang ở cổng 8081 hiển thị tin.

**Dọn dẹp sau khi thử** [THAY ĐỔI TRẠNG THÁI — chỉ xoá đồ tạm, kiểm tra kỹ tên trước khi chạy]:
```sh
docker rm -f nr-restore-app nr-restore-db
docker network rm nr-restore-test
```
Tên phải đúng là `nr-restore-…`. **Không** gõ nhầm thành `news-reader-db-1`.

### D.6. Cập nhật phiên bản

**Làm ở đâu:** máy chạy Docker, thư mục project. **Mức:** [THAY ĐỔI DỮ LIỆU] (có thể có migration).

1. **Ghi phiên bản hiện tại** [CHỈ ĐỌC]:
   ```sh
   git log -1 --oneline
   git status --short
   ```
   Lưu lại mã commit (7 ký tự đầu). `git status` có dòng nào là có thay đổi chưa commit: hỏi người phụ trách trước khi cập nhật.
2. **Sao lưu** (D.2) và **kiểm tra** (D.3).
3. **Lấy code mới** (cách lấy tuỳ người cung cấp; nếu qua Git): `git pull`. [THAY ĐỔI TRẠNG THÁI với mã nguồn]
4. **Xem có migration mới không** [CHỈ ĐỌC]:
   ```sh
   git diff --stat <ma-commit-cu> HEAD -- migrations/ .env.example compose.yaml
   ```
   Có file mới trong `migrations/` ⇒ cấu trúc database sẽ thay đổi khi khởi động. Có thay đổi `.env.example` ⇒ có thể cần thêm biến vào `.env`.
5. **Build và chạy**:
   ```sh
   docker compose up --build -d
   docker compose logs --tail 30 app
   ```
6. **Kiểm tra:** log có `migration applied: …` (nếu có migration mới) và `listening on :8080`; `docker compose ps` `healthy`; mở trang đọc tin; đăng nhập quản trị; xem thẻ nguồn.

### D.7. Quay lại phiên bản trước (rollback) và giới hạn của nó

**Quan trọng:** quay lại code cũ **không** quay lại cấu trúc database. Migration đã chạy thì bảng/cột mới vẫn còn.
- Các migration hiện có chỉ **thêm** (bảng, cột, chỉ mục) và không xoá bài, nên code cũ thường vẫn chạy trên database mới. **Không đảm bảo** cho mọi phiên bản tương lai.
- Một số thay đổi không tương thích ngược: ví dụ code trước ngày 2026-10-06 yêu cầu biến `ADMIN_TOKEN` và không biết tài khoản quản trị.

Cách quay lại code (Chưa kiểm chứng như một quy trình hoàn chỉnh; làm cùng người hỗ trợ):
```sh
git checkout <ma-commit-cu>
docker compose up --build -d
```
Nếu code cũ không chạy được trên database mới, phải **khôi phục database** từ bản sao lưu trước cập nhật (D.8) — mất các bài nhận sau thời điểm sao lưu.

### D.8. Khôi phục đè lên database thật

> **[NGUY HIỂM — CÓ THỂ MẤT DỮ LIỆU]**
> Thao tác này **thay thế** dữ liệu hiện tại bằng dữ liệu trong file sao lưu. Mọi bài/nguồn/thay đổi sau thời điểm sao lưu sẽ mất.
> **Trạng thái: CHƯA KIỂM THỬ trong project.** Chỉ làm khi: (1) đã sao lưu dữ liệu **hiện tại** (D.2) dù nó đang lỗi; (2) đã khôi phục thử thành công file cần dùng (D.5); (3) người chịu trách nhiệm xác nhận **bằng văn bản** cho lần làm này; (4) có người hỗ trợ kỹ thuật cùng làm.

Trình tự đề xuất (Chưa kiểm chứng):
```sh
# 0. Sao lưu hiện trạng (dù đang lỗi)
docker compose exec -T db pg_dump -U news -Fc news > backups/truoc-khi-restore-$(date +%Y%m%d-%H%M).dump
# 1. Dừng app để không ai ghi vào database
docker compose stop app
# 2. [NGUY HIỂM] Khôi phục đè: --clean xoá đối tượng hiện có rồi tạo lại từ file
docker compose exec -T db pg_restore -U news -d news --clean --if-exists --no-owner < backups/TEN-FILE.dump
# 3. Chạy lại app (migration của phiên bản code hiện tại sẽ tự áp dụng nếu bản sao lưu cũ hơn)
docker compose start app
docker compose logs --tail 30 app
```
**Kiểm tra:** số bài, danh sách nguồn, đăng nhập quản trị (tài khoản quản trị cũng trở về trạng thái trong bản sao lưu).

**Không** dùng `docker compose down -v` để "làm sạch" trước khi khôi phục, trừ khi người hỗ trợ yêu cầu và đã có bản sao lưu đã kiểm chứng.

---

## E. Mẫu nhờ hỗ trợ và dùng AI

### E.1. Che thông tin nhạy cảm trước khi gửi

**Không bao giờ gửi:** nội dung file `.env`, mật khẩu, giá trị cookie (`nr_admin`, `__Host-nr_admin`), mã CSRF, file sao lưu `.dump`, hash mật khẩu trong bảng `admin_users`, địa chỉ email cá nhân, địa chỉ IP công khai của VPS (nếu không cần).

**Được gửi:** đoạn log đã đọc qua (che địa chỉ IP nếu cần), kết quả `docker compose ps`, mã commit, URL bài/feed của báo (là thông tin công khai), ảnh chụp màn hình không có dữ liệu cá nhân.

### E.2. Mẫu báo lỗi

```text
TIÊU ĐỀ: [ngắn gọn, ví dụ: "Feed BBC World báo HTTP 403 từ 9h sáng"]

1. Hiện tượng:
   - Thấy gì (chép nguyên văn thông báo):
   - Ở màn hình nào / địa chỉ nào:
2. Thời điểm: [ngày giờ, múi giờ]; lặp lại hay một lần:
3. URL liên quan (nếu có): [URL nguồn/feed/bài gốc]
4. Môi trường:
   - Local hay VPS:
   - Hệ điều hành, trình duyệt:
   - Phiên bản: [kết quả `git log -1 --oneline`]; có thay đổi chưa commit? [kết quả `git status --short` có/không]
5. Các bước tái hiện:
   1.
   2.
   Kết quả mong đợi:
   Kết quả thực tế:
6. Trạng thái container: [dán kết quả `docker compose ps`]
7. Log (đã che thông tin nhạy cảm): [dán ~30 dòng quanh thời điểm lỗi, từ `docker compose logs --since 30m app`]
8. Đã thử: [liệt kê thao tác đã làm và kết quả]
9. Đã sao lưu chưa: [có/không, tên file — KHÔNG gửi file]
```

### E.3. Prompt mẫu khi nhờ AI điều tra/sửa lỗi

Dán prompt sau cho trợ lý AI (có quyền đọc project hoặc không), rồi dán mẫu báo lỗi E.2 bên dưới.

```text
Bạn là kỹ sư hỗ trợ cho project "news-reader" (Go + PostgreSQL + Docker Compose, frontend HTML/JS thuần).

Trước khi đề xuất bất cứ điều gì:
1. Đọc docs/HUONG-DAN-VAN-HANH-VA-XU-LY-LOI.md và docs/GIAI-THICH-HE-THONG-VA-KY-THUAT.md,
   rồi đọc phần code liên quan (cmd/server/, internal/news/, web/, migrations/, compose.yaml).
   Nếu tài liệu và code khác nhau, tin vào code và nói rõ chỗ khác.
2. Điều tra bằng thao tác CHỈ ĐỌC trước (docker compose ps, logs có giới hạn, câu lệnh SELECT, curl tới healthz/feed).
   Nêu bằng chứng (dòng log, kết quả lệnh) cho mỗi kết luận. Phân loại: lỗi người dùng, cấu hình,
   hạ tầng, ứng dụng, hay giới hạn của website nguồn.
3. Chọn cách sửa ít tác động nhất. Giải thích trước khi làm: sẽ thay đổi gì, có ảnh hưởng dữ liệu không, cách hoàn tác.

Ràng buộc bắt buộc:
- Không đọc, in hay yêu cầu tôi dán nội dung .env, mật khẩu, cookie, token, hash mật khẩu, file .dump.
- Không xoá dữ liệu, không chạy `docker compose down -v`, `docker volume rm/prune`, `DROP`, `DELETE`, `TRUNCATE`,
  không khôi phục đè database nếu tôi chưa xác nhận riêng cho lần đó.
- Không tắt hoặc nới lỏng bảo mật (đăng nhập quản trị, CSRF, CSP, kiểm tra URL/IP của worker) để "cho chạy được".
- Không vượt CAPTCHA, giả Referer, vượt đăng nhập/paywall hay cơ chế chặn của website nguồn.
- Trước migration hoặc bất kỳ thay đổi dữ liệu nào: yêu cầu sao lưu (pg_dump) và kiểm tra file sao lưu.
- Chỉ sửa trong phạm vi lỗi; không thêm tính năng, không đổi cấu hình không liên quan.
- Không tự commit, push hay deploy public. Không mở app ra LAN/Internet.
- Sau khi sửa: chạy kiểm tra phù hợp (go vet, go test, node --test tests/, build Docker, thao tác lại bước tái hiện)
  và báo cáo kết quả thật, kể cả khi thất bại. Ghi lại đã đổi file nào và vì sao.
- Nếu bạn KHÔNG truy cập được máy/môi trường của tôi: hãy hướng dẫn tôi từng lệnh kiểm tra (ghi rõ lệnh chỉ đọc
  hay có tác động) và đợi kết quả; không được nói là đã sửa hoặc đã kiểm tra.

Mô tả lỗi:
[dán mẫu báo lỗi E.2 ở đây]
```

### E.4. Khi nào dừng ngay và gọi người hỗ trợ

- Bất kỳ lệnh nào trong nhóm [NGUY HIỂM — CÓ THỂ MẤT DỮ LIỆU].
- Log database có `PANIC`, `corrupted`, `invalid page`, `No space left on device`.
- Lỗi migration.
- Nghi có người lạ đăng nhập (dòng `admin login from` vào giờ bạn không dùng).
- Không chắc lệnh sắp chạy có xoá dữ liệu không.

---

## Thông tin biên soạn

- Ngày biên soạn: 2026-10-06.
- Commit nền: `18f40ce`. Tại thời điểm biên soạn, thư mục làm việc có **thay đổi chưa commit** (lượt cập nhật ngày 2026-10-06: lọc theo quốc gia của nguồn, đọc công khai, tài khoản quản trị, migration `012_countries.sql` và `013_admin_auth.sql`, cùng các file tài liệu). Tài liệu này mô tả phiên bản **gồm** các thay đổi đó.
- Các lệnh ghi "Đã dùng trong project" được chạy trên máy phát triển macOS (Docker qua Colima). Lệnh ghi "Chưa kiểm chứng" (VPS, SSH tunnel, scp, rollback, khôi phục đè) chưa được chạy thử.
- Không có số liệu kiểm thử nào trong tài liệu này được đo lại khi biên soạn; số liệu cũ được ghi kèm ngày đo.
