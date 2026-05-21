# GoClaw — Hướng dẫn Deploy (All-in-one)

Triển khai GoClaw với stack đầy đủ dùng file `docker-compose.all.yml`.  
Stack bao gồm: **GoClaw + PostgreSQL (pgvector) + Headless Chrome + Chrome Proxy + VNC**.

> File này bổ sung cho `README.md` gốc, tập trung vào quy trình build local với `Dockerfile.claude`.

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
│  │  │ :5900    │  │   goclaw-claude:latest     │    │   │
│  │  └──────────┘  └───────────────────────────┘    │   │
│  └──────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────┘
```

| Service        | Image                                | Vai trò                                  |
| -------------- | ------------------------------------ | ---------------------------------------- |
| `goclaw-db`    | `pgvector/pgvector:pg18`             | PostgreSQL với pgvector extension        |
| `chrome`       | `chromedp/headless-shell:latest`     | Headless Chrome, CDP tại `:9222`         |
| `chrome-proxy` | `nginx:alpine`                       | Proxy CDP WebSocket cho goclaw           |
| `vnc`          | `dorowu/ubuntu-desktop-lxde-vnc`     | Desktop GUI xem Chrome qua noVNC `:6080` |
| `goclaw`       | `goclaw-claude:latest` (local build) | App chính + Claude CLI                   |

---

## Dockerfile.claude

Image `goclaw-claude` được build từ `ghcr.io/nextlevelbuilder/goclaw:full`, bổ sung thêm:

- `postgresql18-client` — psql CLI để debug DB
- `xdotool`, `scrot`, `xdpyinfo` — công cụ X11/GUI automation
- `@anthropic-ai/claude-code` — Claude CLI (`claude`)

```dockerfile
FROM ghcr.io/nextlevelbuilder/goclaw:full
RUN apk add postgresql18-client xdotool scrot xdpyinfo \
    && npm install -g @anthropic-ai/claude-code
```

---

## Cài đặt lần đầu

### 1. Chuẩn bị `.env`

```bash
cp .env.example .env
```

Điền các giá trị bắt buộc:

| Biến                    | Mô tả                              | Bắt buộc |
| ----------------------- | ---------------------------------- | -------- |
| `GOCLAW_GATEWAY_TOKEN`  | Token xác thực API gateway         | ✅       |
| `GOCLAW_ENCRYPTION_KEY` | Key mã hóa credentials             | ✅       |
| `POSTGRES_PASSWORD`     | Mật khẩu PostgreSQL                | ✅       |
| `VNC_PASSWORD`          | Mật khẩu VNC (mặc định: `goclaw`)  | ❌       |
| `GOCLAW_PORT`           | Port dashboard (mặc định: `18790`) | ❌       |
| `CHROME_CDP_PORT`       | Port CDP Chrome (mặc định: `9222`) | ❌       |

> 💡 Dùng script `prepare-env.sh` để tự động sinh `GOCLAW_GATEWAY_TOKEN` và `GOCLAW_ENCRYPTION_KEY`:
>
> ```bash
> bash prepare-env.sh
> ```

### 2. Đảm bảo claude CLI đã đăng nhập trên host

Image goclaw mount `/root/.claude` từ host vào container (read-only).  
Claude CLI trong container sẽ dùng session đăng nhập này.

```bash
# Đăng nhập claude trên máy host trước
claude auth login
```

### 3. Tạo external network (nếu chưa có)

```bash
docker network create baota_net
```

### 4. Build và khởi động

```bash
docker compose up -d --build
```

### 5. Kiểm tra

```bash
docker compose ps
docker compose logs -f goclaw
```

Truy cập dashboard: **http://\<HOST_IP\>:18790**

---

## Các lệnh thường dùng

### Khởi động / Dừng

```bash
# Khởi động
docker compose up -d

# Dừng (giữ dữ liệu)
docker compose down

# Dừng và xóa volumes (MẤT DỮ LIỆU)
docker compose down -v
```

### Update GoClaw lên version mới

> ⚠️ **Phải pull base image trước** — Docker sẽ dùng cache cũ nếu bỏ qua bước này.

```bash
# 1. Pull base image mới nhất từ GitHub Container Registry
docker pull ghcr.io/nextlevelbuilder/goclaw:full

# 2. Rebuild image local và restart
docker compose up -d --build
```

### Xem log

```bash
# Log goclaw chính
docker compose logs -f goclaw

# Log tất cả services
docker compose logs -f
```

### Restart một service

```bash
docker compose restart goclaw
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
| `x11-socket`       | X11 socket chia sẻ giữa vnc ↔ goclaw |

---

## Troubleshooting

### Chrome không kết nối được

```bash
# Kiểm tra chrome health
docker compose ps chrome

# Xem log chrome-proxy
docker compose logs chrome-proxy
```

### Claude CLI lỗi authentication

```bash
# Đăng nhập lại claude trên host
claude auth login

# Restart goclaw để nhận session mới
docker compose restart goclaw
```

### Lỗi "network baota_net not found"

```bash
docker network create baota_net
docker compose up -d
```

### Xem version goclaw đang chạy

```bash
docker exec -it $(docker compose ps -q goclaw) goclaw version
```
