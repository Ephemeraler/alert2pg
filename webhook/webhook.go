// Package webhook 用于接收 Alertmanager 推送来的报警信息, 并更新到 Buffer 中.
package webhook

import (
	"alert2pg/buffer"
	"alert2pg/pkg/alert"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Server struct {
	r       *mux.Router
	server  *http.Server
	buffer  *buffer.Buffer
	options Options
	logger  log.Logger

	webhookRequestHistogram    *prometheus.HistogramVec
	webhookAlertCountHistogram prometheus.Histogram
}

func New(buffer *buffer.Buffer, logger log.Logger, opts ...optionFunc) (*Server, error) {
	if buffer == nil {
		return nil, fmt.Errorf("空指针: buffer")
	}

	if logger == nil {
		logger = log.NewNopLogger()
	}

	// 注册路由
	router := mux.NewRouter()

	// 初始化 Receiver 对象.
	s := &Server{
		r:      router,
		server: &http.Server{},
		buffer: buffer,

		logger:  logger,
		options: defaultOptions,

		webhookRequestHistogram: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "alert2pg",
				Subsystem: "webhook",
				Name:      "request_duration_seconds",
				Help:      "Duration of webhook requests in seconds",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{"code"},
		),
		webhookAlertCountHistogram: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Namespace: "alert2pg",
				Subsystem: "webhook",
				Name:      "received_alert_count",
				Help:      "Number of alerts received per webhook request successfully",
				Buckets:   prometheus.ExponentialBuckets(1, 2, 8), // 1, 2, 4, 8, 16, ..., 128
			},
		),
	}

	for _, opt := range opts {
		opt.apply(&s.options)
	}

	router.HandleFunc("/webhook", s.postWebhook).Methods("POST")
	router.Handle("/metrics", promhttp.Handler())

	return s, nil
}

// Run 启动 webhook server 服务.
func (s *Server) Run() error {
	level.Info(s.logger).Log("消息", "启动 webhook server", "服务地址", s.options.address)
	s.server.Handler = s.r
	s.server.Addr = s.options.address

	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		level.Error(s.logger).Log("消息", "启动 webhook server 失败", "错误详情", err)
		return fmt.Errorf("failed to start webhook: %w", err)
	}

	return nil
}

// Stop 停止 webhook server 服务.
func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), s.options.gracePeriod)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		level.Error(s.logger).Log("消息", "无法关闭 webhook server", "错误详情", err)
		return
	}
	level.Info(s.logger).Log("消息", "webhook server 已停止")
}

func (s *Server) postWebhook(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer r.Body.Close()

	// 读取并解析请求体中的报警数据
	body, err := io.ReadAll(r.Body)
	if err != nil {
		level.Error(s.logger).Log("消息", "无法读取请求体", "错误详情", err)
		s.webhookRequestHistogram.WithLabelValues("400").Observe(time.Since(start).Seconds())
		http.Error(w, fmt.Sprintf("无法读取请求体: %s", err), http.StatusBadRequest)
		return
	}

	var ag alert.AlertGroup
	if err := json.Unmarshal(body, &ag); err != nil {
		level.Error(s.logger).Log("消息", "无效的请求体", "错误详情", err)
		s.webhookRequestHistogram.WithLabelValues("400").Observe(time.Since(start).Seconds())
		http.Error(w, fmt.Sprintf("无效的请求体: %s", err), http.StatusBadRequest)
		return
	}

	if ag.Version != s.options.supportVersion {
		level.Error(s.logger).Log("msg", "Invalid payload", "err", fmt.Sprintf("webhook version %s is not supported", ag.Version))
		s.webhookRequestHistogram.WithLabelValues("400").Observe(time.Since(start).Seconds())
		http.Error(w, fmt.Sprintf("Invalid payload: webhook version '%s' is not supported", ag.Version), http.StatusBadRequest)
		return
	}

	// 放入 buffer
	if err := s.buffer.Update(r.Context(), ag.Alerts); err != nil {
		level.Error(s.logger).Log("消息", "更新 Buffer 失败", "错误详情", err)
		s.webhookRequestHistogram.WithLabelValues("500").Observe(time.Since(start).Seconds())
		http.Error(w, "更新 Buffer 失败: 内部处理超时", http.StatusInternalServerError)
		return
	}

	s.webhookAlertCountHistogram.Observe(float64(len(ag.Alerts)))
}
