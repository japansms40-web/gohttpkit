# gohttpkit 质量门禁入口。合入前一键：make check

BASE_REV ?= HEAD~1

.PHONY: build vet test char race lint lint-new tidy-check examples check

build:
	go build ./...

vet:
	go vet ./...

# 全量单元测试。日志级别调到 error，免得 http_transaction 日志刷屏盖住失败信息。
test:
	HTTPKIT_LOG_LEVEL=error go test -count=1 ./...

# 行为锁定：拦截器链的对外行为（重试次数/退避/白名单/头大小写/缓存时机/链序）。
# 改动 httpx 的链结构之前先跑它。
char:
	HTTPKIT_LOG_LEVEL=error go test -count=1 -v -run 'Test(DoRequest|Headers|Retry|Chain|SpecialHeaders|Snapshot|BodyDecode|StatusSemantics|Classify|HTMLText|Transaction|DefaultChain|OnResponseHeaders)' ./httpx/

# 并发回归门禁。需要 C 编译器（-race 依赖 cgo）。
race:
	CGO_ENABLED=1 HTTPKIT_LOG_LEVEL=error go test -race -count=1 ./...

lint:
	golangci-lint run --timeout 5m

# 增量门禁：只对相对 BASE_REV 的改动严格
lint-new:
	golangci-lint run --timeout 5m --new-from-rev=$(BASE_REV)

tidy-check:
	go mod tidy -diff

# 三个示例都能跑起来（fidelity/customchain 自带假服务器，不需要外网）
examples:
	HTTPKIT_LOG_LEVEL=error go run ./examples/customchain
	HTTPKIT_LOG_LEVEL=error go run ./examples/fidelity

check: build vet test tidy-check
