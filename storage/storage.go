// Package storage 负责将 Buffer 中的报警数据高效的持久化到数据库中.
package storage

import (
	"alert2pg/buffer"
	"alert2pg/pkg/alert"
	"context"
	"fmt"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
)

type Storage struct {
	buffer *buffer.Buffer

	pool *pgxpool.Pool

	done   chan struct{}
	ctx    context.Context
	cancel func()

	options                            Options
	logger                             log.Logger
	unloadAlertsGauge                  prometheus.Gauge
	successStorageCounter              prometheus.Counter
	failedStorageCounter               prometheus.Counter
	storageAlertBatchDurationHistogram prometheus.Histogram
	storageAlertDurationHistogram      prometheus.Histogram
}

func New(buffer *buffer.Buffer, logger log.Logger, opts ...optionFunc) (*Storage, error) {
	ctx, cancel := context.WithCancel(context.Background())
	if logger == nil {
		logger = log.NewNopLogger()
	}

	options := defaultOptions
	for _, opt := range opts {
		opt(&options)
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), &options.cfg)
	if err != nil {
		return nil, fmt.Errorf("无法创建连接池: %w", err)
	}
	{
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			return nil, fmt.Errorf("无法连接数据库: %w", err)
		}
	}

	return &Storage{
		buffer:                buffer,
		pool:                  pool,
		done:                  make(chan struct{}),
		ctx:                   ctx,
		cancel:                cancel,
		options:               options,
		logger:                logger,
		unloadAlertsGauge:     prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "alert2pg", Subsystem: "storage", Name: "unload_alerts_total", Help: "Total number of unloaded alerts"}),
		successStorageCounter: prometheus.NewCounter(prometheus.CounterOpts{Namespace: "alert2pg", Subsystem: "storage", Name: "success_alerts_total", Help: "Total number of successful alerts"}),
		failedStorageCounter:  prometheus.NewCounter(prometheus.CounterOpts{Namespace: "alert2pg", Subsystem: "storage", Name: "failed_alerts_total", Help: "Total number of failed alerts"}),
		storageAlertBatchDurationHistogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "alert2pg",
			Subsystem: "storage",
			Name:      "alert_batch_duration_seconds",
			Help:      "Histogram of storage alert batch duration",
			Buckets:   prometheus.DefBuckets,
		}),
		storageAlertDurationHistogram: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "alert2pg",
			Subsystem: "storage",
			Name:      "alert_duration_seconds",
			Help:      "Histogram of individual alert storage duration",
			Buckets:   prometheus.DefBuckets,
		}),
	}, nil
}

func (s *Storage) Run() {
	defer func() {
		close(s.done)
	}()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
			select {
			case <-s.ctx.Done():
				return
			default:
			}
		}
		start := time.Now()
		alerts := s.buffer.GetUnloads()
		s.unloadAlertsGauge.Set(float64(len(alerts)))
		successes := s.Save(alerts)
		s.buffer.SetLoads(successes)
		s.storageAlertDurationHistogram.Observe(time.Since(start).Seconds())
		time.Sleep(1 * time.Second)
	}
}

func (s *Storage) Stop() {
	s.cancel()
	<-s.done
	// 退出前完成一次存储.
	start := time.Now()
	alerts := s.buffer.GetUnloads()
	s.unloadAlertsGauge.Set(float64(len(alerts)))
	successes := s.Save(alerts)
	s.buffer.SetLoads(successes)
	s.storageAlertDurationHistogram.Observe(time.Since(start).Seconds())
	s.pool.Close()
}

// Save 将报警信息持久化到数据库中, 返回成功持久化到数据库中的报警信息.
func (s *Storage) Save(alerts alert.Alerts) alert.Alerts {
	successAlerts := make(alert.Alerts, 0)
	intermediate := make(chan alert.Alert)
	successesChan := make(chan alert.Alert)
	errChan := make(chan error)
	for i := 0; i < s.options.parallelism; i++ {
		go func() {
			for a := range intermediate {
				start := time.Now()
				if err := s.save(a); err != nil {
					level.Error(s.logger).Log("详情", "无法保存报警信息", "fingerprint", a.Fingerprint, "startsAt", a.StartsAt, "错误详情", err)
					errChan <- err
				} else {
					successesChan <- a
				}
				s.storageAlertDurationHistogram.Observe(time.Since(start).Seconds())
			}
		}()
	}

	for range len(alerts) {
		select {
		case a := <-successesChan:
			s.successStorageCounter.Inc()
			successAlerts = append(successAlerts, a)
		case <-errChan:
			s.failedStorageCounter.Inc()
		}
	}

	return successAlerts
}

// save 将一条报警信息存储到数据库中.
func (s *Storage) save(a alert.Alert) error {
	ctx, cancel := context.WithTimeout(s.ctx, s.options.timeout)
	defer cancel()

	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		level.Error(s.logger).Log("详情", "无法从连接池中获取数据库连接", "错误详情", err)
		return fmt.Errorf("无法从连接池中获取数据库连接: %w", err)
	}
	defer conn.Release()

	// 开启事务
	tx, err := conn.Begin(ctx)
	if err != nil {
		level.Error(s.logger).Log("详情", "无法开始事务", "错误详情", err)
		return fmt.Errorf("无法开始事务: %w", err)
	}
	defer func() {
		// 用 Background 确保最大可能地回滚
		_ = tx.Rollback(context.Background())
	}()

	// 保存整体逻辑
	// 首先检查 Alert 表中是否存在该条报警信息.
	// 若存在则为更新, 更新只需要更新 alert, alertannotation 表即可.
	// 若不存在则为插入.
	id := -1
	if err := tx.QueryRow(ctx, `SELECT id FROM Alert WHERE fingerprint = $1 AND startsAt = $2`, a.Fingerprint, a.StartsAt).Scan(&id); err != nil {
		if err != pgx.ErrNoRows {
			level.Error(s.logger).Log("详情", "无法查询 Alert 表中的报警 ID", "fingerprint", a.Fingerprint, "startsAt", a.StartsAt, "错误详情", err)
			return fmt.Errorf("查询 Alert 表中的报警 ID 失败: %w", err)
		}
	}

	if id == -1 {
		// 插入新的报警信息
		if err := tx.QueryRow(ctx, `
	INSERT INTO Alert (fingerprint, status, startsAt, endsAt, generatorURL)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING id`, a.Fingerprint, a.Status, a.StartsAt, a.EndsAt, a.GeneratorURL).Scan(&id); err != nil {
			level.Error(s.logger).Log("详情", "无法在 Alert 表中插入报警信息", "错误详情", err)
			return fmt.Errorf("保存报警数据失败: %w", err)
		}

		for k, v := range a.Labels {
			if _, err := tx.Exec(ctx, `INSERT INTO AlertLabel (AlertID, Label, Value)
			VALUES ($1, $2, $3)`, id, k, v); err != nil {
				level.Error(s.logger).Log("详情", "无法插入 AlertLabel 表中的标签", "key", k, "错误详情", err)
				return fmt.Errorf("保存报警标签数据失败: %w", err)
			}
		}
	} else {
		// 更新现有报警信息
		if _, err := tx.Exec(ctx, `UPDATE Alert SET status = $1, endsAt = $2, generatorURL = $3 WHERE fingerprint = $4 and startsat = $5 `, a.Status, a.EndsAt, a.GeneratorURL, a.Fingerprint, a.StartsAt); err != nil {
			level.Error(s.logger).Log("详情", "无法更新 Alert 表中的报警信息", "fingerprint", a.Fingerprint, "startsAt", a.StartsAt, "错误详情", err)
			return fmt.Errorf("更新 Alert 表中的报警信息失败: %w", err)
		}
	}

	for k, v := range a.Annotations {
		_, err := tx.Exec(ctx, `
		INSERT INTO AlertAnnotation (AlertID, Annotation, Value)
		VALUES ($1, $2, $3)
		ON CONFLICT (AlertID, Annotation) DO UPDATE
		SET Value = EXCLUDED.Value`,
			id, k, v)
		if err != nil {
			level.Error(s.logger).Log("详情", "无法插入或更新注释", "key", k, "错误详情", err)
			return fmt.Errorf("保存报警数据失败: %w", err)
		}
	}

	// 提交事务
	if err := tx.Commit(context.Background()); err != nil {
		level.Error(s.logger).Log("详情", "无法提交事务", "错误详情", err)
		return fmt.Errorf("无法提交事务: %w", err)
	}
	return nil
}
