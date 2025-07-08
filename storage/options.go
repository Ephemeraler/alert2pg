package storage

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var defaultOptions = Options{
	timeout: 5 * time.Second,
}

type Options struct {
	cfg         pgxpool.Config
	timeout     time.Duration // 执行存储一条报警信息的超时时间
	parallelism int
}

type Option interface {
	apply(*Options)
}

type optionFunc func(*Options)

func (f optionFunc) apply(o *Options) {
	f(o)
}
