# GoClaw - Hướng dẫn Deploy (All-in-one)

Triển khai GoClaw với stack đầy đủ dùng file `docker-compose.yaml`.
Stack bao gồm: **GoClaw + PostgreSQL (pgvector) + Headless Chrome + Chrome Proxy + VNC**.

> File này bổ sung cho `README.md` gốc. Stack kéo image dựng sẵn từ Docker Hub, không build local.

> ⚠️ Mọi lệnh dưới đây đều có `-f docker-compose.yaml`. Repo còn một
> `docker-compose.yml` khác cho mục đích khác, và khi cả hai cùng tồn tại thì
> `docker compose` trần chọn `docker-compose.yml`, kèm cảnh báo "Found multiple
> config files". Trên máy prod chỉ có `docker-compose.yaml` thì bỏ `-f` được.

---

## Kiến trúc

```
┌─────────────────────────────────────────────────────────┐
│                      baota_net                          │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │                 goclaw-internal                  │   │
│  │                                                  │   │
│  │  ┌──────────┐  ┌────────┐  ┌────────────────┐   │   │
│  │  │ goclaw-db│  │ chrome │  │  chrome-proxy  │   │   │
│  │  │ :5432    │  │ :9222  │  │  (nginx:alpine)│   │   │
│  │  └──────────┘  └────────┘  └────────────────┘   │   │
│  │                                                  │   │
│  │  ┌──────────┐  ┌───────────────────────────┐    │   │
│  │  │   vnc    │  │          goclaw            │    │   │
│  │  │ :6080    │  │     :18790 (dashboard)     │    │   │
│  │  │ :5900    │  │  thotam/goclaw:beta-full   │    │   │
│  │  └──────────┘  └───────────────────────────┘    │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

| Service        | Image                            | Vai trò                                  |
| -------------- | -------------------------------- | ---------------------------------------- |
| `goclaw-db`    | `pgvector/pgvector:pg18`         | PostgreSQL với pgvector extension        |
| `chrome`       | `chromedp/headless-shell:latest` | Headless Chrome, CDP nội bộ `:9222`      |
| `chrome-proxy` | `nginx:alpine`                   | Proxy CDP WebSocket cho goclaw           |
| `vnc`          | `dorowu/ubuntu-desktop-lxde-vnc` | Desktop GUI xem Chrome qua noVNC `:6080` |
| `goclaw`       | `thotam/goclaw:beta-full`        | App chính, đã kèm Claude CLI             |

---

## Image goclaw

Kéo thẳng từ Docker Hub, **không build local**. Biến thể `-full` đã chứa sẵn:

- `postgresql18-client` - psql 18 để debug DB, khớp version với `goclaw-db`
- `xdotool`, `scrot`, `xdpyinfo` - công cụ X11 dùng chung X display với container `vnc`
- `@anthropic-ai/claude-code` - Claude CLI (`claude`)
- `ffprobe` và `pdfinfo` - hai binary mà hàng rào ngân sách media dùng để đo, thiếu chúng thì `read_video` và `read_audio` từ chối gọi

Đổi image bằng biến `GOCLAW_IMAGE` trong `.env` nếu cần ghim version:

```bash
GOCLAW_IMAGE=thotam/goclaw:v3.15.0-beta.totaa.18-full
```

> Vì sao Docker Hub chứ không phải ghcr.io: cùng một image được đẩy lên cả hai
> registry sau mỗi lần release, nhưng ghcr.io tải chậm hơn đáng kể từ VN.
> Danh sách tag: https://hub.docker.com/r/thotam/goclaw/tags

---

## Cài đặt lần đầu

### 1. Chuẩn bị `.env`

```bash
cp .env.example .env
```

Điền các giá trị bắt buộc:

| Biến                    | Mô tả                                    | Bắt buộc |
| ----------------------- | ---------------------------------------- | -------- |
| `GOCLAW_GATEWAY_TOKEN`  | Token xác thực API gateway               | ✅       |
| `GOCLAW_ENCRYPTION_KEY` | Key mã hóa credentials                   | ✅       |
| `POSTGRES_PASSWORD`     | Mật khẩu PostgreSQL                      | ✅       |
| `GOCLAW_IMAGE`          | Image goclaw (mặc định `:beta-full`)     | ❌       |
| `VNC_PASSWORD`          | Mật khẩu VNC (mặc định: `goclaw`)        | ❌       |
| `GOCLAW_PORT`           | Port dashboard (mặc định: `18790`)       | ❌       |

> 💡 Dùng script `prepare-env.sh` để tự động sinh `GOCLAW_GATEWAY_TOKEN` và `GOCLAW_ENCRYPTION_KEY`:
>
> ```bash
> bash prepare-env.sh
> ```
>
> Script không điền `POSTGRES_PASSWORD`, phải tự đặt.

### 2. Đảm bảo claude CLI đã đăng nhập trên host

Compose mount `/root/.claude` từ host vào container (read-only).
Claude CLI trong container dùng session đăng nhập này.

```bash
# Đăng nhập claude trên máy host trước
claude auth login
```

### 3. Tạo external network (nếu chưa có)

```bash
docker network create baota_net
```

### 4. Kéo image và khởi động

```bash
docker compose -f docker-compose.yaml pull
docker compose -f docker-compose.yaml up -d
```

### 5. Kiểm tra

```bash
docker compose -f docker-compose.yaml ps
docker compose -f docker-compose.yaml logs -f goclaw
```

Truy cập dashboard: **http://\<HOST_IP\>:18790**

---

## Các lệnh thường dùng

### Khởi động / Dừng

```bash
# Khởi động
docker compose -f docker-compose.yaml up -d

# Dừng (giữ dữ liệu)
docker compose -f docker-compose.yaml down

# Dừng và xóa volumes (MẤT DỮ LIỆU)
docker compose -f docker-compose.yaml down -v
```

### Update GoClaw lên version mới

Không còn bước build. Kéo image mới rồi dựng lại container:

```bash
docker compose -f docker-compose.yaml pull goclaw
docker compose -f docker-compose.yaml up -d goclaw
```

> Nếu `GOCLAW_IMAGE` đang ghim một version cụ thể thì `pull` sẽ không lấy bản mới.
> Sửa version trong `.env` trước, hoặc bỏ ghim để dùng `:beta-full`.

### Xem log

```bash
# Log goclaw chính
docker compose -f docker-compose.yaml logs -f goclaw

# Log tất cả services
docker compose -f docker-compose.yaml logs -f
```

### Restart một service

```bash
docker compose -f docker-compose.yaml restart goclaw
```

---

## Truy cập VNC (xem GUI Chrome)

| Giao thức       | URL                  | Mô tả                       |
| --------------- | -------------------- | --------------------------- |
| noVNC (browser) | `http://<HOST>:6080` | Xem desktop qua trình duyệt |
| VNC Client      | `<HOST>:5900`        | Dùng VNC Viewer             |

Mật khẩu: giá trị `VNC_PASSWORD` trong `.env` (mặc định: `goclaw`)

---

## Volumes

| Volume             | Nội dung                             |
| ------------------ | ------------------------------------ |
| `goclaw-data`      | Config, credentials, database nội bộ |
| `goclaw-workspace` | Workspace làm việc                   |
| `goclaw-skills`    | Skills/plugins                       |
| `postgres-data`    | Dữ liệu PostgreSQL                   |
| `x11-socket`       | X11 socket chia sẻ giữa vnc và goclaw |

---

## Troubleshooting

### Chrome không kết nối được

Cổng CDP **không publish ra host**, goclaw nói chuyện với `chrome-proxy` qua network
`goclaw-internal`. Muốn debug từ máy ngoài thì bỏ comment khối `ports` của service
`chrome` trong `docker-compose.yaml`.

```bash
# Kiểm tra chrome health
docker compose -f docker-compose.yaml ps chrome

# Xem log chrome-proxy
docker compose -f docker-compose.yaml logs chrome-proxy
```

### Claude CLI lỗi authentication

```bash
# Đăng nhập lại claude trên host
claude auth login

# Restart goclaw để nhận session mới
docker compose -f docker-compose.yaml restart goclaw
```

### Lỗi "network baota_net not found"

```bash
docker network create baota_net
docker compose -f docker-compose.yaml up -d
```

### `read_video` hoặc `read_audio` báo không đo được thời lượng

Hai tool này cần `ffprobe` để tính trước chi phí token. Biến thể `-full` có sẵn,
nhưng nếu đổi `GOCLAW_IMAGE` sang biến thể `base` thì sẽ thiếu:

```bash
docker exec -it $(docker compose -f docker-compose.yaml ps -q goclaw) ffprobe -version
```

Không có thì quay lại image `-full` hoặc `:beta`.

### Xem version goclaw đang chạy

```bash
docker exec -it $(docker compose -f docker-compose.yaml ps -q goclaw) goclaw version
```

### Kiểm tra psql trong container

```bash
docker exec -it $(docker compose -f docker-compose.yaml ps -q goclaw) psql --version
```
