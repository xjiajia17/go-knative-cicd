// Command server 是一个刻意保持简单的业务服务，用来串起
// GitHub Actions -> 华为云 SWR -> Argo CD -> Knative -> Istio -> Prometheus 整条链路。
//
// 它只做三件事：
//   - GET /         返回一段 JSON（含当前 Revision / Pod / 版本，方便肉眼确认发版是否生效）
//   - GET /healthz  健康检查
//   - GET /metrics  Prometheus 抓取入口，与业务共用同一个端口
//
// 为什么共用端口：Knative Serving 规定一个容器只能声明一个 containerPort，
// 再声明第二个端口会被 webhook 直接拒绝。所以 /metrics 必须挂在业务端口上。
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/xjiajia17/go-knative-cicd/internal/metrics"
)

// 由 -ldflags "-X main.version=... -X main.commit=..." 在构建时注入。
var (
	version = "dev"
	commit  = "none"
)

type indexResponse struct {
	Message  string `json:"message"`
	Service  string `json:"service"`            // Knative 注入的 K_SERVICE
	Revision string `json:"revision"`           // Knative 注入的 K_REVISION，发版后会变
	Pod      string `json:"pod"`                // HOSTNAME，缩容到 0 再冷启动会换名字
	Version  string `json:"version"`            // 镜像里烧进去的版本
	Commit   string `json:"commit"`             // 对应的 git commit
	DelayMS  int    `json:"delay_ms,omitempty"` // 人为注入的延迟，用来演示 P95 曲线
}

func main() {
	port := envOr("PORT", "8080") // Knative 会自动注入 PORT，默认 8080
	target := envOr("TARGET", "Knative")

	metrics.SetBuildInfo(version, commit)

	mux := http.NewServeMux()
	// Go 1.22 起的路由模式："/{$}" 只精确匹配根路径，不会吞掉 /metrics 等。
	mux.HandleFunc("GET /{$}", handleIndex(target))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.Handle("GET /metrics", metrics.Handler())

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           metrics.Middleware(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// 优雅退出：Knative 缩容/换 Revision 时先发 SIGTERM，queue-proxy 会先摘流量再等进程退出。
	// 不处理 SIGTERM 的话，正在处理的请求会被硬切断。
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("listening on :%s version=%s commit=%s", port, version, commit)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
	log.Println("bye")
}

func handleIndex(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ?delay=200 人为制造 200ms 延迟，上限 2s，方便在 Grafana 上看 P95 变化。
		delayMS := 0
		if v := r.URL.Query().Get("delay"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				delayMS = min(n, 2000)
				time.Sleep(time.Duration(delayMS) * time.Millisecond)
			}
		}

		resp := indexResponse{
			Message:  "Hello " + target + "!",
			Service:  os.Getenv("K_SERVICE"),
			Revision: os.Getenv("K_REVISION"),
			Pod:      os.Getenv("HOSTNAME"),
			Version:  version,
			Commit:   commit,
			DelayMS:  delayMS,
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}