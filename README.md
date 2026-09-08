# ABP Bot TikTok

TikTok crawler bot viết bằng Go với Playwright, crawl kết quả search công khai của TikTok qua GPM (GoLogin Profile Manager) và đẩy dữ liệu ra một backend API bên ngoài.

## Cấu trúc

```
├── cmd/
│   └── main.go           # Entry point chính
├── internal/
│   ├── crawler/          # Orchestrator + GPM circuit breaker, scraper, searcher, publisher
│   ├── models/           # Data models (VideoItem)
│   ├── parser/           # Convert VideoItem -> payload gửi API (TiktokPost)
│   ├── repository/       # Đọc keyword (JSON file / PostgreSQL — xem ghi chú bên dưới)
│   ├── scheduler/        # Vòng lặp crawl định kỳ (random interval)
│   └── utils/            # Utilities (scroll, delay, retry, resource monitor)
├── pkg/
│   ├── api/               # HTTP client đẩy dữ liệu lên backend
│   ├── config/            # Config loader (.env)
│   ├── database/          # PostgreSQL client (hiện chưa được gọi ở main.go)
│   ├── gpm/                # HTTP client điều khiển GoLogin Profile Manager
│   └── logger/            # Zap logger
├── configs/
│   └── keywords.json      # Danh sách keyword crawl (nguồn keyword hiện tại)
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

### 5. PostgreSQL
`POSTGRES_DSN` là biến **bắt buộc phải điền**. Khi vào `main()`, bot kết nối PostgreSQL và chạy một truy vấn một lần để lấy URL các bài viết đã có sẵn trong `tbl_posts` (bảng thuộc backend, không phải bảng do bot quản lý):

```sql
SELECT tp.url FROM tbl_posts tp
 WHERE tp.org_id = $ORG_ID AND tp.crawl_source_code = 'tt' AND tp.pub_time > $PUB_TIME
```

`ORG_ID` và `PUB_TIME` (unix seconds) cũng là biến **bắt buộc**, dùng để lọc truy vấn này (xem [internal/repository/post_repo.go](internal/repository/post_repo.go)). Nếu PostgreSQL không kết nối được, bước này chỉ log lỗi (không fatal) — vòng crawl chính vẫn chạy bình thường vì nó đọc keyword từ `configs/keywords.json`, không phụ thuộc bước này.

Phần code đọc **keyword** từ PostgreSQL (bảng `keyword`) vẫn đang bị **comment out** trong `cmd/main.go` — không liên quan tới truy vấn `tbl_posts` ở trên.

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

# Backend API nhận dữ liệu crawl (để trống = crash khi có video cần đẩy — xem mục "Đẩy dữ liệu")
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

# PostgreSQL pool (chỉ áp dụng nếu sau này bật lại phần kết nối Postgres)
# POSTGRES_MAX_POOL_SIZE=100
# POSTGRES_MIN_POOL_SIZE=10

# HTTP client gọi API_URL
# HTTP_TIMEOUT_SECONDS=30

# Danh sách keyword fallback qua env (hiện KHÔNG được main.go dùng — xem mục "Nguồn keyword")
# KEYWORDS=keyword1,keyword2

# Nghỉ giữa các keyword trong cùng 1 batch (giây)
# SLEEP_MIN_KEYWORD=60
# SLEEP_MAX_KEYWORD=180

# Nghỉ giữa các batch/session (giây)
# REST_MIN_SESSION=180
# REST_MAX_SESSION=300

# Số keyword gom trong 1 batch trước khi nghỉ session
# BATCH_MIN=3
# BATCH_MAX=5

# Giới hạn chống crawl quá đà
# MAX_VIDEOS_PER_KEYWORD=200
# MAX_PAGES_PER_SESSION=20
```

### Nguồn keyword
Bot **không** đọc keyword từ PostgreSQL ở thời điểm hiện tại (đoạn code đó bị comment trong `cmd/main.go`), và cũng không đọc từ biến env `KEYWORDS`. Nguồn keyword thực tế là file `configs/keywords.json` — một mảng JSON các chuỗi keyword, ví dụ:
```json
["Xã Xuân Giang", "phường Láng Hạ"]
```
File JSON không phân biệt org, nên mọi keyword đọc từ đây đều được gán cố định `org_id = 0` (hardcode trong `cmd/main.go`) khi đẩy dữ liệu ra API.

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
2. Chạy ngay 1 chu kỳ crawl toàn bộ keyword.
3. Sau khi xong, nghỉ một khoảng **ngẫu nhiên 30–45 phút** rồi lặp lại bước 1.

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

GPM cho phép sử dụng (các) browser profile đã login TikTok sẵn, tránh phải login lại mỗi lần chạy. Bot hỗ trợ chạy **nhiều profile song song** qua `PROFILE_IDS` — mỗi profile crawl một phần keyword riêng (chia round-robin), profile sau được khởi động lệch ngẫu nhiên 15–45s so với profile trước.

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

### Cách bot hoạt động khi có GPM (mỗi batch keyword)
1. Gọi GPM API để start profile (retry tối đa 5 lần nếu browser chưa sẵn sàng)
2. Lấy `ws_endpoint` (hoặc `remote_debugging_address` rồi query CDP `/json/version`)
3. Connect Playwright qua CDP (Chrome DevTools Protocol) — có circuit breaker: mở sau 3 lần lỗi liên tiếp, tự thử lại sau 5 phút, retry kết nối với backoff 1s/2s/4s
4. Sử dụng browser đã login sẵn để search từng keyword trong batch
5. Sau khi crawl xong batch, đóng browser và gọi GPM để stop profile
6. Nghỉ ngẫu nhiên rồi lặp lại cho batch tiếp theo

## Đẩy dữ liệu ra ngoài

Bot **không ghi trực tiếp vào PostgreSQL**. Video crawl được đưa vào một buffer nội bộ (3 worker, gom tối đa 10 video hoặc mỗi 5 giây) rồi `POST` theo batch tới:

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
      "description": "...",
      "url": "https://www.tiktok.com/@username/video/7123456789",
      "auth_id": "123456",
      "auth_name": "Display Name",
      "comments": 100,
      "shares": 50,
      "reactions": 1000,
      "favors": 200,
      "views": 10000,
      "pub_time": 1714089600,
      "crawl_time": 1714090000,
      "crawl_source_code": "tt",
      "crawl_bot": "tiktok-1",
      "...": "xem đầy đủ field ở internal/parser/tiktok_post.go"
    }
  ]
}
```

Retry 2 lần (có backoff) nếu request lỗi; nếu vẫn thất bại thì log warning và bỏ batch đó (không chặn crawl tiếp).

**Quan trọng:** nếu `API_URL` để trống, bot vẫn khởi động và crawl bình thường nhưng sẽ lỗi (`nil pointer`) ngay khi có video đầu tiên cần đẩy đi. Luôn cấu hình `API_URL` trỏ tới backend thật khi chạy production.

Khi channel buffer đầy (backend chậm/API_URL sai), video mới sẽ bị **drop kèm log warning** thay vì bị block — đây là cơ chế backpressure chủ động, không phải lỗi.

## Tính năng

- ✅ Multi-profile GPM chạy song song, tự chia keyword round-robin
- ✅ Circuit breaker cho kết nối GPM (closed/open/half-open) + retry backoff
- ✅ Intercept TikTok search API (`/api/search/item/full/`)
- ✅ Human-like behavior (scroll, mouse move, random view video)
- ✅ Theo dõi CPU/RAM, tạm dừng crawl nếu máy quá tải (>80%)
- ✅ Né khung giờ đêm (00:00–03:00), chạy lại lúc 03:00
- ✅ Giới hạn video/keyword và số trang/session để tránh crawl quá đà
- ✅ Đẩy dữ liệu theo batch tới backend API, có backpressure khi API chậm
- ✅ Structured logging (Zap), có session ID riêng cho mỗi lần chạy profile

## Anti-ban

Code đã có:
- Random sleep giữa keywords (mặc định 60–180s, cấu hình qua `SLEEP_MIN/MAX_KEYWORD`)
- Random rest giữa các batch/session (mặc định 180–300s, cấu hình qua `REST_MIN/MAX_SESSION`)
- Stagger khởi động giữa các profile (15–45s)
- Human scroll simulation
- Random mouse movement
- Random video viewing
- Tự tạm dừng 5 phút khi phát hiện rate-limit/captcha từ response API

Khuyến nghị thêm:
- Chỉ chạy 7h-23h (tránh 2h-6h sáng)
- Giới hạn ~50-100 keywords/ngày
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

### Không thấy video nào được crawl / danh sách keyword rỗng
→ Bot đọc keyword từ `configs/keywords.json`, không phải PostgreSQL. Kiểm tra file này tồn tại và có ít nhất 1 keyword hợp lệ.

### Video crawl được nhưng không thấy dữ liệu ở backend / bot crash khi đẩy API
→ Kiểm tra `API_URL` đã được điền đúng và trỏ tới backend đang chạy, endpoint `POST /api/v1/posts/insert-unclassified-org-posts` trả về status < 400.

## Deploy lên server khác

1. Copy toàn bộ project
2. Cài Playwright driver (xem mục Cài đặt)
3. Sửa `.env` với config đúng (đặc biệt `GPM_API`, `PROFILE_IDS`, `API_URL`)
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
