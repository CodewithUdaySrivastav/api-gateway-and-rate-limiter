Markdown
# 🚀 High-Performance Distributed API Gateway & Rate Limiter

A production-grade, asynchronous API Gateway and Reverse Proxy built from scratch in **Go (Golang)**, featuring distributed token-bucket rate limiting via **Redis**, dynamic tier management, and non-blocking background telemetry logging.

---

## 🏗️ Architecture & Request Flow

[ Client Request ]
│ (Header: X-API-Key: sk_live_...)
▼
[ Go API Gateway (Reverse Proxy / Multiplexer) ]
├── 1. Thread-Safe Auth Check (Validates API Key & Tier)
├── 2. Distributed Rate Limiter (Atomic Redis INCR + TTL)
├── 3. Reverse Proxy Engine (Forwards with Injected Headers)
└── 4. Asynchronous Analytics Worker (Non-blocking Channel Logging)


---

## ✨ Key Features & Engineering Highlights

* **High-Throughput Reverse Proxy:** Built using Go's native `net/http/httputil` package to seamlessly proxy incoming traffic to downstream microservices with minimal latency.
* **Distributed Rate Limiting:** Utilizes **Redis** to maintain atomic request counters across distributed windows, returning standard `429 Too Many Requests` headers with `Retry-After` metadata when limits are breached.
* **Dynamic Tier-Based Quotas:** Supports role-based limits (e.g., *Free Tier*: 5 req/min, *Pro Tier*: 20 req/min) managed through thread-safe in-memory maps with mutex locks.
* **Asynchronous Telemetry & Logging:** Implements Go channels (`chan RequestLog`) and background worker goroutines to process and record request metrics asynchronously, ensuring logging never blocks client response times.
* **Dynamic Admin Control Plane:** Exposes a secure REST endpoint (`POST /admin/keys`) to provision new API keys and configure rate limits on the fly without restarting the server.

---

## 🛠️ Tech Stack
* **Language:** Go (Golang)
* **Caching & Quota Store:** Upstash Redis (RESP Protocol)
* **Networking:** Go Standard Library (`net/http`, `httputil`)
* **Concurrency:** Goroutines, Channels, `sync.RWMutex`

---

## 📦 Getting Started & Running Locally

1. **Clone the repository:**
   ```bash
   git clone [https://github.com/your-username/api-gateway.git](https://github.com/your-username/api-gateway.git)
   cd api-gateway
Configure your Redis Environment Variable:
Set your Upstash or local Redis connection string in your terminal session:

PowerShell
$env:REDIS_URL="rediss://default:YOUR_PASSWORD@your-redis-endpoint:6379"
Run the Gateway:

Bash
go run main.go
Test an Authenticated Request:

Bash
curl.exe -i -H "X-API-Key: sk_live_12345" http://localhost:8080/users/123

