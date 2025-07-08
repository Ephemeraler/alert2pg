package buffer

import "time"

var (
	defaultOptions = Options{
		// TODO 测试一下 Resolved 的报警 Alertmanager 会重复发送多少次, 发送间隔是多少?
		maxLifetime:  10 * time.Minute,
		syncInterval: 1 * time.Second,
		gcInterval:   5 * time.Minute,
	}
)

type Options struct {
	alertmanagerAddr string
	maxLifetime      time.Duration
	syncInterval     time.Duration
	gcInterval       time.Duration
}
