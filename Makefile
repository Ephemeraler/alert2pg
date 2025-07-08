# 读取 VERSION 文件中的内容作为版本号
VERSION := $(shell cat VERSION)
# 构建输出文件名
BINARY := alert2pg
# 源文件目录
MAIN := ./cmd/main.go

# 构建参数，传入 version 变量
BUILD_FLAGS := -ldflags "-X 'main.Version=$(VERSION)'"

.PHONY: all build run clean version

# 默认目标
all: build

# 构建可执行文件
build:
	go build $(BUILD_FLAGS) -o $(BINARY) $(MAIN)

# 运行程序
run:
	go run $(BUILD_FLAGS) $(MAIN)

# 清理编译产物
clean:
	rm -f $(BINARY)

# 打印版本信息
version:
	@echo $(VERSION)
