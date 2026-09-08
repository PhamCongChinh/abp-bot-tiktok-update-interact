# ABP Bot TikTok

Bot viết bằng Go với Playwright, dùng GPM (GoLogin Profile Manager) để mở lần lượt các URL video TikTok đã biết (lấy từ bảng `tbl_posts` của backend) và scrape lại số liệu hiện tại (views/likes/comments/shares...) của từng video.

## Cấu trúc

```
├── cmd/
│   └── main.go           # Entry point chính
├── internal/
│   ├── crawler/          # Orchestrator + GPM circuit breaker, scraper, visitor (visit URL), publisher
│   ├── models/           # Data models (VideoItem)
│   ├── parser/           # Convert VideoItem -> payload gửi API (TiktokPost)
│   ├── repository/       # Truy vấn PostgreSQL (tbl_posts, keyword/bot_config — xem ghi chú bên dưới)
│   ├── scheduler/        # Vòng lặp crawl định kỳ (random interval)
│   └── utils/            # Utilities (scroll, delay, retry, resource monitor)
├── pkg/
│   ├── api/               # HTTP client đẩy dữ liệu lên backend (hiện chưa được gọi — xem "Đẩy dữ liệu ra ngoài")
│   ├── config/            # Config loader (.env)
│   ├── database/          # PostgreSQL client
│   ├── gpm/                # HTTP client điều khiển GoLogin Profile Manager
│   └── logger/            # Zap logger
└── data/                  # Log output (OUTPUT_DIR)
```

## Cài đặt

### 1. Cài Go (>= 1.21)
```bash
# Kiểm tra
go version
```

### 2. Clone project & cài dependencies
```bash
git clone <repo-url>
cd abp-bot-tiktok
go mod download
```

### 3. Cài Playwright driver
```bash
go run github.com/playwright-community/playwright-go/cmd/playwright@v0.5700.1 install chromium
```

### 4. GPM (GoLogin Profile Manager) — bắt buộc
Bot **luôn cần GPM để chạy** (không có chế độ dùng local Chrome trực tiếp trong code hiện tại — nếu `GPM_API`/`PROFILE_IDS` thiếu, mỗi chu kỳ crawl sẽ bị bỏ qua với log lỗi). Xem chi tiết cài đặt ở phần "GPM Setup" bên dưới.

### 5. PostgreSQL — bắt buộc
`POSTGRES_DSN` là biến bắt buộc. Khi vào `main()`, bot kết nối PostgreSQL và chạy một truy vấn để lấy URL các bài viết đã có sẵn trong `tbl_posts` (bảng thuộc backend, không phải bảng do bot quản lý):

```sql
SELECT tp.url FROM tbl_posts tp
 WHERE tp.org_id = $ORG_ID AND tp.crawl_source_code = 'tt' AND tp.pub_time > $PUB_TIME
```

`ORG_ID` và `PUB_TIME` (unix seconds) cũng là biến **bắt buộc**, dùng để lọc truy vấn này (xem [internal/repository/post_repo.go](internal/repository/post_repo.go)). Danh sách URL trả về là **nguồn crawl target duy nhất** của bot — nếu PostgreSQL không kết nối được, hoặc query lỗi, hoặc không có URL nào khớp, bot sẽ thoát ngay (log Fatal/Warn tương ứng), vì không còn nguồn nào khác để crawl.

## Cấu hình

### 1. Tạo `.env`
```bash
cp .env.example .env
```

### 2. Các biến bắt buộc
```env
BOT_NAME=bot-tiktok-01
POSTGRES_DSN=postgres://postgres:postgres@localhost:5432/sls_tiktok?sslmode=disable
GPM_API=http://localhost:50325/api/v1
ORG_ID=987700
PUB_TIME=1785517201
```

### 3. Các biến tùy chọn (giá trị mặc định nếu không set)
```env
# GPM — danh sách nhiều profile, phân tách dấu phẩy. Để trống = bot không crawl được gì.
# PROFILE_IDS=profile-1,profile-2

# Backend API nhận dữ liệu crawl (hiện chưa được gọi — xem mục "Đẩy dữ liệu ra ngoài")
# API_URL=http://localhost:8080/api

# Thư mục chứa log
# OUTPUT_DIR=./data

# DEBUG=true -> chạy 1 lần rồi thoát; DEBUG=false -> chạy theo scheduler
# DEBUG=false

# Logging
# LOG_LEVEL=info
# LOG_MAX_SIZE_MB=100
# LOG_MAX_AGE_DAYS=7
# LOG_MAX_BACKUPS=7

# PostgreSQL pool
# POSTGRES_MAX_POOL_SIZE=100
# POSTGRES_MIN_POOL_SIZE=10

# HTTP client gọi API_URL
# HTTP_TIMEOUT_SECONDS=30

# Nghỉ giữa các URL trong cùng 1 batch (giây)
# SLEEP_MIN_KEYWORD=60
# SLEEP_MAX_KEYWORD=180

# Nghỉ giữa các batch/session (giây)
# REST_MIN_SESSION=180
# REST_MAX_SESSION=300

# Số URL gom trong 1 batch (1 browser session) trước khi nghỉ
# BATCH_MIN=3
# BATCH_MAX=5

# Giới hạn số "trang" (batch) tối đa mỗi session, chống crawl quá đà
# MAX_PAGES_PER_SESSION=20
```

## Sử dụng

### Chạy crawler (DEBUG mode, chạy 1 lần)
```bash
go run cmd/main.go
```

### Chạy production (lặp tự động)
```bash
# Sửa .env: DEBUG=false, đảm bảo PROFILE_IDS có ít nhất 1 profile và GPM đang chạy
build.bat
bot.exe
```
Ở chế độ production, bot **không dùng cron biểu thức** — logic lặp nằm trong `internal/scheduler`:
1. Nếu giờ hiện tại nằm trong khoảng 00:00–03:00, bot ngủ tới đúng 03:00 mới bắt đầu (né giờ đêm).
2. Chạy ngay 1 chu kỳ crawl toàn bộ URL đã lấy từ `tbl_posts` lúc khởi động.
3. Sau khi xong, nghỉ một khoảng **ngẫu nhiên 30–45 phút** rồi lặp lại bước 1 (cùng danh sách URL — không truy vấn lại `tbl_posts` giữa các chu kỳ).

## Tự khởi động cùng Windows

Để bot tự chạy mỗi khi đăng nhập Windows (không cần mở tay):

```bash
build.bat
install_startup.bat
```

`install_startup.bat` tạo shortcut trỏ tới `bot.exe` trong thư mục Startup của Windows (`shell:startup`). Bot sẽ tự chạy (kèm cửa sổ console) ngay khi bạn đăng nhập lần sau.

**Gỡ bỏ:** mở Win+R, gõ `shell:startup`, xóa shortcut `abp-bot-tiktok`.

**Lưu ý:** cách này không tự khởi động lại nếu bot bị crash. Nếu cần restart tự động khi lỗi, cân nhắc dùng Task Scheduler thay vì Startup folder.

## GPM Setup (GoLogin Profile Manager)

GPM cho phép sử dụng (các) browser profile đã login TikTok sẵn, tránh phải login lại mỗi lần chạy. Bot hỗ trợ chạy **nhiều profile song song** qua `PROFILE_IDS` — mỗi profile visit một phần danh sách URL riêng (chia round-robin), profile sau được khởi động lệch ngẫu nhiên 15–45s so với profile trước.

### Cài đặt GPM
1. Download GPM: https://gologin.com/
2. Cài đặt và mở GPM
3. Tạo (các) profile mới hoặc dùng profile có sẵn
4. Login TikTok trong (các) profile đó

### Lấy thông tin GPM
- **API URL**: kiểm tra trong GPM Settings → API (mặc định thường là `http://127.0.0.1:19995/api/v3` hoặc `http://localhost:50325/api/v1` tùy phiên bản GPM)
- **Profile ID**: mở GPM → click vào profile muốn dùng → copy Profile ID từ URL hoặc profile settings. Nhiều profile thì nối bằng dấu phẩy trong `PROFILE_IDS`.

Kiểm tra GPM đang chạy:
```bash
curl <GPM_API>/profiles
```
Nếu lỗi → mở GPM trước khi chạy bot.

### Cách bot hoạt động khi có GPM (mỗi batch URL)
1. Gọi GPM API để start profile (retry tối đa 5 lần nếu browser chưa sẵn sàng)
2. Lấy `ws_endpoint` (hoặc `remote_debugging_address` rồi query CDP `/json/version`)
3. Connect Playwright qua CDP (Chrome DevTools Protocol) — có circuit breaker: mở sau 3 lần lỗi liên tiếp, tự thử lại sau 5 phút, retry kết nối với backoff 1s/2s/4s
4. Với browser đã login sẵn: mở trang chủ TikTok (warm-up), rồi `Goto` trực tiếp từng URL video trong batch
5. Trên mỗi trang video: bắt response XHR `/api/item_detail/` (fallback: đọc script `#__UNIVERSAL_DATA_FOR_REHYDRATION__` nếu không bắt được XHR) để lấy `id/desc/createTime/author/stats`
6. Sau khi visit xong batch, đóng browser và gọi GPM để stop profile
7. Nghỉ ngẫu nhiên rồi lặp lại cho batch tiếp theo

## Đẩy dữ liệu ra ngoài

**Hiện tại bot chỉ log dữ liệu scrape được, chưa đẩy đi đâu.** Mỗi video visit thành công được log ở mức Info với đầy đủ `video_id/views/comments/shares/reactions/favors/author` (xem `internal/crawler/visitor.go`).

Hạ tầng đẩy batch lên backend (`internal/crawler/publisher.go`, `pkg/api/client.go`) vẫn còn nguyên — 3 worker gom tối đa 10 video hoặc mỗi 5 giây rồi `POST` tới:

```
POST {API_URL}/api/v1/posts/insert-unclassified-org-posts
Content-Type: application/json

{
  "index": "not_classify_org_posts",
  "upsert": true,
  "data": [
    {
      "org_id": 2,
      "subject_id": "7123456789",
      "url": "https://www.tiktok.com/@username/video/7123456789",
      "comments": 100,
      "shares": 50,
      "reactions": 1000,
      "favors": 200,
      "views": 10000,
      "...": "xem đầy đủ field ở internal/parser/tiktok_post.go"
    }
  ]
}
```

Nhưng luồng visit-URL hiện tại **chưa gọi tới** `Publisher.PushBatch` — cần nối thêm khi có yêu cầu cập nhật số liệu về backend.

## Tính năng

- ✅ Multi-profile GPM chạy song song, tự chia danh sách URL round-robin
- ✅ Circuit breaker cho kết nối GPM (closed/open/half-open) + retry backoff
- ✅ Lấy danh sách URL crawl target từ `tbl_posts` (PostgreSQL) theo `ORG_ID`/`PUB_TIME`
- ✅ Intercept TikTok video-detail API (`/api/item_detail/`), fallback đọc JSON nhúng trong trang (`__UNIVERSAL_DATA_FOR_REHYDRATION__`)
- ✅ Human-like behavior (scroll, mouse move, warm-up ở trang chủ trước khi vào video)
- ✅ Theo dõi CPU/RAM, tạm dừng crawl nếu máy quá tải (>80%)
- ✅ Né khung giờ đêm (00:00–03:00), chạy lại lúc 03:00
- ✅ Giới hạn số batch/session để tránh crawl quá đà
- ✅ Structured logging (Zap), có session ID riêng cho mỗi lần chạy profile

## Anti-ban

Code đã có:
- Random sleep giữa các URL (mặc định 60–180s, cấu hình qua `SLEEP_MIN/MAX_KEYWORD`)
- Random rest giữa các batch/session (mặc định 180–300s, cấu hình qua `REST_MIN/MAX_SESSION`)
- Stagger khởi động giữa các profile (15–45s)
- Human scroll simulation
- Random mouse movement
- Warm-up ở trang chủ trước khi vào từng video
- Tự tạm dừng 5 phút khi phát hiện rate-limit/captcha từ response API

Khuyến nghị thêm:
- Chỉ chạy 7h-23h (tránh 2h-6h sáng)
- Dùng profile GPM đã có lịch sử duyệt web thật
- Rotate IP nếu crawl nhiều

## Troubleshooting

### Lỗi: "please install the driver (v1.57.0) first"
→ Chạy lại:
```bash
go run github.com/playwright-community/playwright-go/cmd/playwright@v0.5700.1 install chromium
```

### Lỗi: "GPM config required. Set GPM_API and PROFILE_IDS in .env"
→ `PROFILE_IDS` đang trống. Điền ít nhất 1 profile ID, phân tách dấu phẩy nếu nhiều profile.

### Lỗi: "No connection could be made..." khi gọi GPM_API
→ GPM chưa chạy hoặc sai `GPM_API`. Mở GPM trước, kiểm tra lại port trong GPM Settings → API.

### Lỗi: "failed to start profile" / "empty remote_debugging_address"
→ Kiểm tra `PROFILE_IDS` đúng chưa, hoặc thử start profile thủ công trong GPM trước.

### Lỗi: "gpm circuit breaker open: N consecutive failures, reset in ..."
→ Kết nối GPM đã lỗi liên tiếp 3 lần, bot tạm ngừng thử trong 5 phút để tránh spam. Kiểm tra GPM đang chạy ổn định, chờ circuit tự chuyển sang half-open rồi bot sẽ thử lại.

### Bot thoát ngay khi khởi động, log "No post URLs found, exiting"
→ Truy vấn `tbl_posts` với `ORG_ID`/`PUB_TIME` hiện tại không trả về URL nào. Kiểm tra lại 2 giá trị này (đặc biệt `PUB_TIME` — quá lớn sẽ lọc hết dữ liệu cũ).

### Không thấy video nào scrape được / log toàn "no video data extracted"
→ TikTok có thể đã đổi cấu trúc response `/api/item_detail/` hoặc script `__UNIVERSAL_DATA_FOR_REHYDRATION__` — cần capture lại network trace thực tế và cập nhật `internal/crawler/visitor.go`.

## Deploy lên server khác

1. Copy toàn bộ project
2. Cài Playwright driver (xem mục Cài đặt)
3. Sửa `.env` với config đúng (đặc biệt `GPM_API`, `PROFILE_IDS`, `POSTGRES_DSN`)
4. Chạy `go run cmd/main.go`

**Hoặc build binary trên máy dev rồi copy:**
```bash
# Máy dev
build.bat

# Copy bot.exe + .env sang server
# Chạy trên server
bot.exe
```

**Lưu ý:** Playwright driver phải cài trên từng máy riêng!

## License

MIT
