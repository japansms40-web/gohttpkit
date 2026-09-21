# gohttpkit 质量门禁入口。
#   make check  —— 本地轻量：build + vet + cover + tidy（不含 lint / race，避免每次要 cgo）
#   合入口径    —— check + lint-new + race（CI 已跑后两项；本地完整请 make ci）

BASE_REV ?= HEAD~1

.PHONY: build vet test char race cover cover-html lint lint-new tidy-check examples check ci

# 覆盖率门禁：低于 MIN_COVERAGE 直接失败。数字只能往上调，不许往下调——
# 往下调的那一刻，这道门禁就从"防线"变成了"摆设"。
MIN_COVERAGE ?= 90

build:
	go build ./...

vet:
	go vet ./...

# 全量单元测试。httpx_test 的 TestMain 把默认日志压到 error，避免 event=http.transaction 刷屏。
test:
	go test -count=1 ./...

# 行为锁定：拦截器链的对外行为（重试次数/退避/白名单/头大小写/缓存时机/链序）。
# 改动 httpx 的链结构之前先跑它。
char:
	go test -count=1 -v -run 'Test(DoRequest|Headers|Retry|Chain|SpecialHeaders|Snapshot|BodyDecode|StatusSemantics|Classify|HTMLText|Transaction|DefaultChain|OnResponseHeaders|Logging)' ./httpx/

# 并发回归门禁。需要 C 编译器（-race 依赖 cgo）。
race:
	CGO_ENABLED=1 go test -race -count=1 ./...

# 覆盖率报告 + 门禁
cover:
	@go test -count=1 -coverprofile=coverage.out ./... > /dev/null
	@go tool cover -func=coverage.out | tail -1
	@total=$$(go tool cover -func=coverage.out | tail -1 | grep -oE '[0-9]+\.[0-9]+'); \
	 pass=$$(awk -v t="$$total" -v m="$(MIN_COVERAGE)" 'BEGIN{print (t+0 >= m+0) ? "1" : "0"}'); \
	 if [ "$$pass" != "1" ]; then echo "覆盖率 $$total% 低于门禁 $(MIN_COVERAGE)%"; exit 1; fi; \
	 echo "覆盖率 $$total% ≥ 门禁 $(MIN_COVERAGE)% ✓"

# 在浏览器里看逐行覆盖情况（排查"这行到底有没有被跑到"）
cover-html: cover
	go tool cover -html=coverage.out

lint:
	golangci-lint run --timeout 5m

# 增量门禁：只对相对 BASE_REV 的改动严格
lint-new:
	golangci-lint run --timeout 5m --new-from-rev=$(BASE_REV)

tidy-check:
	go mod tidy -diff

# 两个离线示例（fidelity/customchain 自带假服务器，不需要外网）。
# quickstart 默认请求真实 URL（httpbin.org），需要外网，故不在这里跑：go run ./examples/quickstart
examples:
	go run ./examples/customchain
	go run ./examples/fidelity

check: build vet cover tidy-check

# 本地「我要完整跑」与 CI 对齐的聚合目标。race 需要 C 编译器。
ci: check lint-new race char
