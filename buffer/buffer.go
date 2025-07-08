// package buffer 作为报警数据缓冲区
// 负责减少 webhook 接收重复报警而导致的重复存储行为
// 负责定期同步 Alertmanager 与 Buffer 中的 Firing 报警数据, 减少由于到达顺序的问题导致的状态不一致
package buffer

import (
	"alert2pg/pkg/alert"
	"alert2pg/pkg/http"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"golang.org/x/sync/semaphore"
)

type Buffer struct {
	buffer map[string]*alert.Alert
	sem    *semaphore.Weighted

	wg     sync.WaitGroup
	done   chan struct{}
	ctx    context.Context
	cancel context.CancelFunc

	logger log.Logger

	options Options
}

// Run 启动运行 Buffer Sync 与 Gc 任务.
func (b *Buffer) Run() {
	b.wg.Add(2)
	go b.Sync()
	go b.Gc()
	b.wg.Wait()
	close(b.done)
}

func (b *Buffer) Stop() {
	b.cancel()
	<-b.done
	// 退出前完成一次同步.
	b.sync()
}

// GetUnloads 获取 Buffer 中所有为持久化到数据库中的报警信息.
func (b *Buffer) GetUnloads() alert.Alerts {
	alerts := make(alert.Alerts, 0)
	return alerts
}

// DeepCopy 深拷贝 Buffer 中的报警信息.
func (b *Buffer) DeepCopy() alert.Alerts {
	b.Lock(context.Background())
	defer b.Unlock()

	alerts := make(alert.Alerts, 0, len(b.buffer))
	for _, a := range b.buffer {
		alerts = append(alerts, *a)
	}
	return alerts
}

// SetLoads 将加载到数据库中的报警信息标记为已加载
func (b *Buffer) SetLoads(alerts alert.Alerts) {
	b.Lock(context.Background())
	defer b.Unlock()

	for _, a := range alerts {
		if source, ok := b.buffer[a.Key()]; ok && source.Equal(a) {
			source.Loaded = true
			source.LoadedAt = time.Now()
		}
	}
}

// Update 更新 Buffer 中报警信息, 重复报警不会更新标志位.
func (b *Buffer) Update(ctx context.Context, alerts alert.Alerts) error {
	if err := b.Lock(ctx); err != nil {
		level.Error(b.logger).Log("描述", "获取 Buffer 锁失败", "err", err)
		return fmt.Errorf("获取 Buffer 锁失败: %w", err)
	}
	defer b.Unlock()

	for _, a := range alerts {
		// TODO: 这里存在不安全性, 可能会存在 Map 中不存在的 key.
		if a.Equal(*b.buffer[a.Key()]) {
			// 报警信息相同时
			b.buffer[a.Key()].LoadedAt = a.LoadedAt
		} else {
			// 报警信息不一致时
			b.buffer[a.Key()] = &a
		}
	}
	return nil
}

func (b *Buffer) Sync() {
	ticker := time.NewTicker(b.options.syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := b.sync(); err != nil {
				level.Error(b.logger).Log("描述", "同步 Alertmanager 与 Buffer 中的报警信息失败", "err", err)
			}
		case <-b.ctx.Done():
			return
		}
	}
}

func (b *Buffer) sync() error {
	alerts, err := http.GetFiringAlertsFromAlertmanager(b.options.alertmanagerAddr, true, false, false, false)
	if err != nil {
		return fmt.Errorf("无法同步 Alertmanager 与 Buffer 中的报警信息: %w", err)
	}

	b.Lock(context.Background())
	defer b.Unlock()

	set := make(map[string]struct{}, len(alerts))
	for _, a := range alerts {
		set[a.Key()] = struct{}{}
	}

	for key, a := range b.buffer {
		if _, ok := set[key]; !ok && a.Status == alert.Firing {
			a.SetResolved()
		}

	}
	return nil
}

func (b *Buffer) Gc() {
	ticker := time.NewTicker(b.options.gcInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.gc()
		case <-b.ctx.Done():
			return
		}
	}
}

// gc 回收超期报警信息.
func (b *Buffer) gc() {
	b.Lock(context.Background())
	defer b.Unlock()

	for key, a := range b.buffer {
		if a.IsExpired(b.options.maxLifetime) {
			delete(b.buffer, key)
		}
	}
}

// Lock 获取 Buffer 锁, 支持通过 ctx 方式控制获取锁等待的时间.
func (b *Buffer) Lock(ctx context.Context) error {
	return b.sem.Acquire(ctx, 1)
}

// Unlock 释放 Buffer 锁.
func (b *Buffer) Unlock() {
	b.sem.Release(1)
}
