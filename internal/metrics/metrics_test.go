package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// 验证三件事：计数正确、未知路径被归一到 other（防基数爆炸）、/metrics 不被计入。
func TestMiddleware(t *testing.T) {
	RequestsTotal.Reset()
	RequestDuration.Reset()

	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/boom" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	hit := func(path string) {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	hit("/")
	hit("/")
	hit("/random-scan-1")
	hit("/random-scan-2")
	hit("/boom")
	hit("/metrics")
	hit("/healthz")

	if got := testutil.ToFloat64(RequestsTotal.WithLabelValues("GET", "/", "200")); got != 2 {
		t.Errorf("根路径计数 = %v, 期望 2", got)
	}
	if got := testutil.ToFloat64(RequestsTotal.WithLabelValues("GET", "other", "200")); got != 2 {
		t.Errorf("未知路径应归一到 other，计数 = %v, 期望 2", got)
	}
	if got := testutil.ToFloat64(RequestsTotal.WithLabelValues("GET", "other", "500")); got != 1 {
		t.Errorf("500 状态码应被记录，计数 = %v, 期望 1", got)
	}
	// /metrics 与 /healthz 被跳过，所以总序列数应该是 3（200根 / 200other / 500other）。
	if got := testutil.CollectAndCount(RequestsTotal); got != 3 {
		t.Errorf("指标序列数 = %d, 期望 3（说明 /metrics 或 /healthz 被误计入）", got)
	}
	if got := testutil.CollectAndCount(RequestDuration); got != 2 {
		t.Errorf("耗时直方图序列数 = %d, 期望 2（按 path 拆分：/ 与 other）", got)
	}
}