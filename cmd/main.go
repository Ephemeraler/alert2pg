package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/run"
)

var (
	Version = "unknown" // 默认值
)

func main() {
	var logger log.Logger

	// 保证退出顺序
	webhookExitChan := make(chan struct{})
	bufferExitChan := make(chan struct{})
	// 1. 解析命令行

	// 2. 创建线程管理器

	// 3. 读取数据库中  firing 报警并添加到 Buffer 中.

	// 4. Buffer 执行 Sync 一次.
	// 服务启动初始化阶段 -> Buffer 加载数据库中 firing 报警 -> Buffer Sync -> Storage 存储操作

	// 开始所有服务.

	// 接收到关闭信息 -> Webhook 服务停止 -> Buffer 执行同步服务 -> Storage 完成全部数据存储 -> 退出程序
	var g run.Group

	// 信号处理
	{
		// 使用有缓冲通道, 防止阻塞系统通知.
		term := make(chan os.Signal, 1)
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				select {
				case sig := <-term:
					level.Warn(logger).Log("消息", "收到系统信号, alert2pg 优雅退出中...", "信号", sig.String())
				case <-cancel:
				}
				return nil
			},
			func(_ error) {
				close(cancel)
			},
		)
	}

	// webhook 服务
	{
		g.Add(
			func() error {
				close(webhookExitChan)
				return nil
			},
			func(_ error) {
				level.Info(logger).Log("消息", "webhook 服务关闭中...")
			},
		)
	}

	// Buffer 服务
	{
		g.Add(
			func() error {
				close(bufferExitChan)
				return nil
			},
			func(err error) {
				level.Info(logger).Log("消息", "Buffer 服务关闭中...等待 webhook 服务退出")
				<-webhookExitChan
				// Buffer 清理相关工作
			},
		)
	}

	// storage 服务
	{
		g.Add(
			func() error {
				return nil
			},
			func(err error) {
				<-bufferExitChan
			},
		)
	}

	if err := g.Run(); err != nil {
		level.Error(logger).Log("消息", "alert2pg 运行失败", "错误", err)
		os.Exit(1)
	}
}
