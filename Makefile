# gohttpkit 质量门禁入口。
#   make hooks  —— clone 后先跑一次：安装本地 git 门禁（.githooks + core.hooksPath）
#   make check  —— 本地轻量：build + vet + cover + tidy（不含 lint / race，避免每次要 cgo）
#   合入口径    —— check + lint-new + race（CI 已跑后两项；本地完整请 make ci）

BASE_REV ?= HEAD~1

.PHONY: build vet test char race cover cover-pkg cover-html lint lint-new tidy-check examples check ci hooks agents-sync-check

# 覆盖率门禁（核心库，排除 examples）：低于 MIN_COVERAGE 直接失败。
# 已达标 98%（补齐 interceptor 角度测试后）；100% 为追求，只许上调、不许下调——
# 往下调的那一刻门禁就从"防线"变成"摆设"。规范见 docs/TESTING.md。
MIN_COVERAGE ?= 98

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

# 覆盖率报告 + 门禁（核心库，排除 examples）
cover:
	@go test -count=1 -coverprofile=coverage.out $$(go list ./... | grep -v '/examples/') > /dev/null
	@go tool cover -func=coverage.out | tail -1
	@total=$$(go tool cover -func=coverage.out | tail -1 | grep -oE '[0-9]+\.[0-9]+'); \
	 pass=$$(awk -v t="$$total" -v m="$(MIN_COVERAGE)" 'BEGIN{print (t+0 >= m+0) ? "1" : "0"}'); \
	 if [ "$$pass" != "1" ]; then echo "覆盖率 $$total% 低于门禁 $(MIN_COVERAGE)%"; exit 1; fi; \
	 echo "覆盖率 $$total% ≥ 门禁 $(MIN_COVERAGE)% ✓"

# 逐包覆盖率：定位哪个包是短板（排除 examples）。规范见 docs/TESTING.md §5。
cover-pkg:
	@go test -count=1 -cover $$(go list ./... | grep -v '/examples/') \
	  | awk '/coverage:/{for(i=1;i<=NF;i++) if($$i=="coverage:") printf "%-48s %s\n", $$2, $$(i+1)}'

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
ci: check lint-new race char agents-sync-check

# 一键安装本地 git 门禁（零第三方依赖，走 .githooks + core.hooksPath）。
# clone 后跑一次即可；升级钩子脚本后无需重装。
hooks:
	@git config core.hooksPath .githooks
	@chmod +x .githooks/*
	@echo "已启用 .githooks：pre-commit / commit-msg / pre-push。关闭：git config --unset core.hooksPath"

# 校验跨 agent 指令三文件同源：CLAUDE.md / GEMINI.md 必须是指向 AGENTS.md 的符号链接。
# 谁把软链改成了实体文件、或内容漂移，这里直接失败。
agents-sync-check:
	@test -f AGENTS.md || { echo "缺少 AGENTS.md（正本）"; exit 1; }
	@for f in CLAUDE.md GEMINI.md; do \
	  test -L "$$f" || { echo "$$f 不是符号链接（应 ln -sf AGENTS.md $$f）"; exit 1; }; \
	  [ "$$(readlink "$$f")" = "AGENTS.md" ] || { echo "$$f 未指向 AGENTS.md"; exit 1; }; \
	done
	@echo "AGENTS.md / CLAUDE.md / GEMINI.md 同源 ✓"
