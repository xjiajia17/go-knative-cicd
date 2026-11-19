// Package metrics 定义业务自定义指标，并提供采集这些指标的 HTTP 中间件。
//
// 设计要点（也是面试/生产里常被问到的）：
//  1. 指标名遵循 Prometheus 规范：<namespace>_<name>_<unit>，计数器以 _total 结尾。
//  2. 标签基数必须可控：path 只允许白名单里的已知路由，其余一律归到 "other"，
//     否则爬虫随便扫几个 URL 就能把 Prometheus 的时序数量打爆。
//  3. /metrics 自身不计入业务访问量，否则 Prometheus 每 10s 一次的抓取会淹没真实流量。
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// RequestsTotal 就是用户要的"访问量"。Grafana 里用
	// sum(rate(demo_http_requests_total[1m])) 画 QPS。
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "demo_http_requests_total",
		Help: "业务请求总数，按 method/path/status 拆分。",
	}, []string{"method", "path", "status"})

	// RequestDuration 用来算 P95：
	// histogram_quantile(0.95, sum(rate(demo_http_request_duration_seconds_bucket[5m])) by (le))
	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "demo_http_request_duration_seconds",
		Help:    "业务请求耗时分布（秒）。",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	}, []string{"method", "path"})

	// BuildInfo 是"信息型指标"的经典写法：值恒为 1，信息全在标签里。
	// 这样在 Grafana 上能直接看出当前跑的是哪个 commit，发版前后一目了然。
	BuildInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "demo_build_info",
		Help: "构建信息，值恒为 1，版本与 commit 在标签中。",
	}, []string{"version", "commit"})
)

// knownPaths 是允许出现在 path 标签里的路由白名单。
var knownPaths = map[string]bool{
	"/":        true,
	"/healthz": true,
}

// skippedPaths 里的请求不计入业务指标。
var skippedPaths = map[string]bool{
	"/metrics": true,
	"/healthz": true,
}

// Handler 返回 /metrics 的处理器，输出 Prometheus 文本格式。
func Handler() http.Handler { return promhttp.Handler() }

// SetBuildInfo 在进程启动时调用一次。
func SetBuildInfo(version, commit string) {
	BuildInfo.WithLabelValues(version, commit).Set(1)
}

// statusRecorder 包一层 ResponseWriter，把状态码记下来。
// 不包的话中间件拿不到 handler 最终写出的状态码。
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Middleware 采集每个请求的计数与耗时。
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if skippedPaths[path] {
			next.ServeHTTP(w, r)
			return
		}

		label := "other"
		if knownPaths[path] {
			label = path
		}

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		RequestsTotal.WithLabelValues(r.Method, label, strconv.Itoa(rec.status)).Inc()
		RequestDuration.WithLabelValues(r.Method, label).Observe(time.Since(start).Seconds())
	})
}