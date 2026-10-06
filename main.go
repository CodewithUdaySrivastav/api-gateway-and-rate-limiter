package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()
var rdb *redis.Client

// API Key Metadata Structure
type APIKeyInfo struct {
	Tier  string `json:"tier"`
	Limit int    `json:"limit"` // Requests per minute
}

// Thread-safe map for storing keys dynamically
var (
	keyMutex     sync.RWMutex
	validAPIKeys = map[string]APIKeyInfo{
		"sk_live_12345": {Tier: "free", Limit: 5},
		"sk_live_99999": {Tier: "pro", Limit: 20},
	}
)

// Analytics Log Structure
type RequestLog struct {
	APIKey       string        `json:"api_key"`
	Tier         string        `json:"tier"`
	Path         string        `json:"path"`
	StatusCode   int           `json:"status_code"`
	ResponseTime time.Duration `json:"response_time"`
	Timestamp    time.Time     `json:"timestamp"`
}

// Buffered channel for asynchronous analytics logging
var logChan = make(chan RequestLog, 1000)

func main() {
	// 1. Read the Redis URL securely from environment variables
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("❌ REDIS_URL environment variable is not set! Run: $env:REDIS_URL=\"rediss://...\" first.")
	}

	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Fatalf("❌ Failed to parse Redis URL: %v", err)
	}
	rdb = redis.NewClient(opt)

	_, err = rdb.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("❌ Failed to connect to Redis: %v", err)
	}
	fmt.Println("📦 Connected to Upstash Cloud Redis successfully!")

	// 2. Start Background Analytics Worker (Async Logging)
	go startAnalyticsWorker()

	// 3. Start Mock Backend Server on port 9000
	go startMockBackend()

	backendURL, err := url.Parse("http://localhost:9000")
	if err != nil {
		log.Fatalf("Invalid backend URL: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(backendURL)

	// 4. Main Router & Gateway Multiplexer
	mux := http.NewServeMux()

	// --- ADMIN ENDPOINT: Create New API Keys Dynamically ---
	mux.HandleFunc("/admin/keys", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req struct {
			Key   string `json:"key"`
			Tier  string `json:"tier"`
			Limit int    `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		keyMutex.Lock()
		validAPIKeys[req.Key] = APIKeyInfo{Tier: req.Tier, Limit: req.Limit}
		keyMutex.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "API key created successfully"})
	})

	// --- GATEWAY PROXY MIDDLEWARE ROUTE ---
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// A. Validate API Key
		apiKey := r.Header.Get("X-API-Key")

		keyMutex.RLock()
		info, exists := validAPIKeys[apiKey]
		keyMutex.RUnlock()

		if !exists {
			log.Printf("[Gateway] UNAUTHORIZED: Invalid or missing API key")
			http.Error(w, "Forbidden: Invalid or missing API Key", http.StatusUnauthorized)
			return
		}

		// B. Dynamic Rate Limiting via Redis
		minuteBucket := time.Now().Format("2006-01-02_15:04")
		redisKey := fmt.Sprintf("rate_limit:%s:%s", apiKey, minuteBucket)

		currentRequests, err := rdb.Incr(ctx, redisKey).Result()
		if err != nil {
			log.Printf("[Redis Error] %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if currentRequests == 1 {
			rdb.Expire(ctx, redisKey, 60*time.Second)
		}

		if currentRequests > int64(info.Limit) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, fmt.Sprintf("429 Too Many Requests: Limit of %d req/min exceeded.", info.Limit), http.StatusTooManyRequests)

			// Log the rate-limited hit asynchronously
			logChan <- RequestLog{
				APIKey:       apiKey,
				Tier:         info.Tier,
				Path:         r.URL.Path,
				StatusCode:   http.StatusTooManyRequests,
				ResponseTime: time.Since(start),
				Timestamp:    time.Now(),
			}
			return
		}

		// C. Inject Headers and Proxy
		r.Header.Set("X-Gateway", "MiniGateway-Go")
		r.Header.Set("X-User-Tier", info.Tier)

		// Custom ResponseWriter wrapper to capture status code for logging
		rw := &statusResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		proxy.ServeHTTP(rw, r)

		// D. Send Telemetry Log to Asynchronous Worker Channel
		logChan <- RequestLog{
			APIKey:       apiKey,
			Tier:         info.Tier,
			Path:         r.URL.Path,
			StatusCode:   rw.statusCode,
			ResponseTime: time.Since(start),
			Timestamp:    time.Now(),
		}
	})

	fmt.Println("🚀 Supercharged API Gateway running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// Background Worker: Processes analytics logs concurrently without blocking traffic
func startAnalyticsWorker() {
	for l := range logChan {
		log.Printf("[Analytics Engine] 📊 Key: %s | Tier: %s | Path: %s | Status: %d | Latency: %v",
			l.APIKey, l.Tier, l.Path, l.StatusCode, l.ResponseTime)
	}
}

// Helper wrapper to capture HTTP status codes from proxy responses
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func startMockBackend() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tier := r.Header.Get("X-User-Tier")
		fmt.Fprintf(w, "Hello from Backend! User Tier: %s\n", tier)
	})
	log.Fatal(http.ListenAndServe(":9000", mux))
}
