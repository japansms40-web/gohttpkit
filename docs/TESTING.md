# 测试规范

> 一句话：**行覆盖率是副产品，不是目标**。真正的门禁是「每个该测的角度都测到了」。
> 一个测试可以跑过 100% 的行、却几乎不断言任何东西——调了函数不看返回、不触发错误分支、
> 不跨边界、不并发。它看起来是绿的，证明不了什么。本规范按**角度**驱动测试，行 % 随之而来。
>
> 配套：效力分级见 [`CODE_STANDARDS.md`](CODE_STANDARDS.md) §8 / §8.1；门禁入口 `Makefile`；治理见 [`ENGINEERING_GOVERNANCE.md`](ENGINEERING_GOVERNANCE.md)。

## 1. 覆盖率政策

- **核心库**（`errors` / `geo` / `httpx` / `logger` / `netproxy` / `traffic` / `versionreg`，即除 `examples/` 外）
  行覆盖率**硬门禁**：`make cover` 低于 `MIN_COVERAGE` 直接失败。
- **目标 ≥ 98%，100% 为追求**。`MIN_COVERAGE`（在 `Makefile`）是当前地板，**只许上调、不许下调**（棘轮）——
  每补齐一个包就把地板往上抬一档，直到 98。下调的那一刻，门禁就从「防线」变成「摆设」。
- **`examples/` 不计入覆盖门禁**（演示代码），但 `make examples` 保证它可编译、可运行。
- **为什么 100% 不设成硬门禁**：硬卡 100% 会逼你为「理论上到不了」的防御分支写假测试，制造绿色假象；
  也会奖励「调一次不断言」的凑数测试。100% 行覆盖 **≠ 测够了**——见 §2。真正到不了的行按 §4 显式豁免。

## 2. 多角度覆盖（本规范的核心）

对每个被测函数，走查下表：**这个角度对这段代码适用吗？适用的话，有没有测试真的触发它？**
大多数函数只沾其中 4–7 个，不是全部十二个。挑「**能真出错**」的角度补，判为 out-of-scope 的角度
要在测试注释或 PR 里写明理由——**沉默省略会被当成「已覆盖」，其实没有**。

| # | 角度 | 它回答的问题 | 缺失的味道 |
|---|------|------------|-----------|
| 1 | happy path | 典型输入产出预期结果？ | 没有一条直白的「正常用例」断言 |
| 2 | **边界** | 0 / 1 / 空 / max / min / off-by-one / 刚好越限 | 只测「中间值」，没有 `len==0`/`len==1` |
| 3 | 错误路径 | 每条 `return err` 都被走到并断言了？ | `err` 被 `_` 丢弃；只测成功 |
| 4 | 错误传播 | 包装后还能认出身份 / 原因？ | 只断言「有错」，不问「是哪个错」 |
| 5 | nil / 零值 | nil 指针、空 slice/map、零值 struct、nil 函数 | 输入永远是填满的 |
| 6 | 并发 | N goroutine 下无竞态？原子？与顺序无关？ | 有共享状态却没有 `-race` 测试 |
| 7 | 契约不变量 | doc comment 承诺的保证真的成立？ | 注释说「只有一个非零」却没人验 |
| 8 | 状态迁移 | 开↔关、set→unset、init 前后、幂等 | 全局/有状态 API 只在一种状态下测 |
| 9 | 副作用 | 断言的是**效果**而非返回值？ | 只断言返回值，忽略它触发的回调/写入/日志 |
| 10 | 资源生命周期 | open/close、双关、泄漏、出错时清理 | `Close()` 从没测过；无泄漏/defer 检查 |
| 11 | 输入多样性 | 表驱动扫代表性组合 | 每个函数一个手挑输入 |
| 12 | 对抗/畸形 | 垃圾、截断、超大、注入、编码 | 只测格式良好的输入 |

**发现隐藏角度的反向推理**（借 `systematic-debugging` 的反向数据流）：不是问「这个 bug 从哪来」，
而是问「**什么输入会让它出错，测了吗？**」——
- 每条 `if`/`switch`/`return` 分支，什么输入能到达它？有没有带那个输入的测试？
- 每个能产出的值（含错误与副作用），产生它的最小输入是什么？漏一个=漏一条测试。
- 每个外部依赖（conn/文件/时钟/nil）：它返回错误、返回 0 字节、阻塞、为 nil 时会怎样？每个都是一个角度。

## 3. 每个测试的硬规矩

- **一个测试一个行为**；命名按行为不按行号：`拒绝空国家码` / `rejects_empty_country`，不是 `Test_validate_2`。
  测试名需要「and」就拆开。
- **边界必测**：0 / 1 / 空 / 满 / 越限各一条（角度 #2），不是只有一个中间值。
- **错误要断言「是哪个错」**（角度 #4）：本库类型错误用 `errors.As` 解出类型和字段，**不只判 `err != nil`**；
  `io.EOF`/`context.Canceled`/第三方哨兵用 `errors.Is`。（与 CODE_STANDARDS §5「类型错误非哨兵」一致。）
- **副作用断言副作用本身**（角度 #9）：回调用捕获的实参断言、写入检查内容、日志检查字段，不只看返回值。
- **并发相关代码**（角度 #6）必须有 `-race` + 多 goroutine 测试，断言**精确**聚合值（不是「大概对」）。
  范例：`traffic` 的 8 goroutine × 100 读写，断言字节总数精确相等。
- **表驱动扫多输入**（角度 #11）：每行断言前 `t.Logf` 一行输入与结果（§8.1），默认 `go test` 不刷屏。
- **零值合法时必须能和失败区分**：如 `geo.TimezoneOffsetForCountry` 的 `0` 是合法 GMT+0，要看 `ok`。

## 4. 豁免机制（真不可达的行）

极少数**真正到不了**的防御分支（如「按类型系统不可能进入的 `default: panic`」）可以豁免，但要显式：

```go
// coverage:ignore  <为什么到不了：如「上游已保证 kind 只有这三种，此分支仅防御未来新增」>
default:
    panic("unreachable: ...")
```

- 豁免是**例外不是常态**。绝大多数「测不到」其实是「还没想到那个输入」——先按 §2 反向推理，别急着豁免。
- **禁止**用「调用一次不断言」来凑行覆盖。凑出来的绿是负资产：它看起来 done，比已知的洞更危险。

## 5. 工作流（MEASURE → ENUMERATE → CLOSE → VERIFY）

```bash
make cover                                   # 门禁：核心库总覆盖率 vs MIN_COVERAGE
make cover-pkg                               # 逐包覆盖率，定位哪个包是短板
go test -coverprofile=/tmp/c.out ./httpx/interceptor/ && go tool cover -func=/tmp/c.out   # 逐函数看洞
go tool cover -html=/tmp/c.out               # 可视化：红=从没跑到
go test -race -count=1 ./httpx/...           # -race 抓角度 #6，-count=1 绕开缓存
```

1. **MEASURE**：把覆盖率报告当成「哪些行跑过」的地图，不是分数。红行是**保证有洞**；跑过但没断言的行是**危险的洞**。
2. **ENUMERATE**：对短板函数走 §2 角度表，列出未覆盖角度作为工作清单。
3. **CLOSE（每个洞走 TDD）**：先写一条 RED 测试看它**因正确的原因失败**（断言不符，不是编译错/拼写错），再让它绿。
   - 测已存在代码时：写对了通常一次就绿；若「该过却失败」——**挖到真 bug，改代码不改测试**。
   - 一次只补一个角度，近似的角度测试最后收进表驱动。
4. **VERIFY**：重测覆盖率，但真正的检查是 §6 的定性清单。

## 6. 一个测试文件的 Definition of Done

- [ ] 每个「适用」的角度要么覆盖、要么显式写明 out-of-scope 理由
- [ ] 每条测试都是先写、先看它失败（TDD），且失败原因符合预期
- [ ] 错误分支断言的是**哪个错**（`errors.As`/`errors.Is`），不只是「非 nil」
- [ ] 副作用被断言（回调带对参数触发了、写入发生了）
- [ ] 契约/doc 不变量有一条「违反就会挂」的断言
- [ ] 并发相关代码有真的起 goroutine 的 `-race` 测试
- [ ] 输出干净——`-race` 下无泄漏日志、无告警、无数据竞争
- [ ] 核心库总覆盖率 ≥ `MIN_COVERAGE`，且本次没让它回退

## 7. 落地节奏（棘轮上调）

不要求一次到 98。按包补：先测短板包（逐函数报告里 < 100% 的先补错误路径与边界），
每让核心库总覆盖率稳定站上一档就把 `Makefile` 的 `MIN_COVERAGE` 抬一档（如 95 → 96 → 97 → 98），
每档在提交说明里写清抬到多少。**只上不下。**

## 8. 泄漏与并发

- **MUST** 启动 goroutine 的包（`httpx`、`netproxy`、`traffic`、`logger`）在 `TestMain` 里接 goleak
  （`goleak.VerifyTestMain(m)`），测试结束仍存活的 goroutine 直接判失败。（待落地，见治理文档 §7。）
- **MUST** 并发测试断言**精确**聚合值，并至少跑一次 `-race -count=1`；需要放大竞态时本地 `-count=50`。
- **MUST NOT** 用 `time.Sleep` 等「大概够了」来同步 goroutine；用 channel / `sync.WaitGroup` / 条件等待。
  确需等待异步效果时用有上限的轮询（如 `require.Eventually` 语义的自写 helper），超时即失败。

## 9. fuzz 与 benchmark

- **SHOULD** 解析外部原文的函数（代理 URL、content-encoding、版本标识、header 白名单、Accept-Language 拼装）配 fuzz。
  范例：`netproxy/proxy_fuzz_test.go`、`httpx/encoding_fuzz_test.go`、`versionreg/registry_fuzz_test.go`。
- **MUST** fuzz 发现的崩溃输入会写入 `testdata/fuzz/FuzzXxx/`，**必须提交**，作为永久回归种子（`go test` 默认会跑）。
- **SHOULD** 本地改到解析逻辑时跑 `go test -fuzz=FuzzXxx -fuzztime=30s ./pkg/`；CI 定时（nightly）跑长时 fuzz（待落地）。
- **SHOULD** 热路径配 `BenchmarkXxx`，带 `b.ReportAllocs()`；对比用 `benchstat`，至少 `-count=6`（见 `CODE_STANDARDS.md` §15）。
- **MUST NOT** 把 benchmark 数值写成断言（机器差异会让它 flaky）；allocs 必须恒定的场景用 `testing.AllocsPerRun` 断言。

## 10. 确定性

- **MUST** 测试离线、可重复、与执行顺序无关：不访问外网（自带 `httptest` / 最小协议实现）、不依赖本机代理、
  不依赖真实时区与 locale、不写仓库目录（用 `t.TempDir()`）。
- **MUST** 依赖时间或随机数的逻辑可注入：时钟用函数 / 接口参数，随机用 `*rand.Rand` 固定种子。
  范例：`geo.BuildChromeAcceptLanguage` 接收 `*rand.Rand`。
- **MUST** flaky 零容忍：发现偶发失败立即开 issue 并修复根因；禁止加重试、放宽断言或 `t.Skip` 了事。
- **SHOULD** 无共享全局状态的测试加 `t.Parallel()`；改全局状态（`logger.SetHandler`、包级变量）的测试**不得**并行，
  并用 `t.Cleanup` 还原。
- **SHOULD** 单个测试 < 1s，包级 < 30s；CI 统一带 `-timeout`（默认 10m）防挂死。
- **SHOULD** golden 文件放 `testdata/`，更新走 `-update` 标志并在 PR 说明为什么变；禁止手改 golden 迁就实现。

## 11. 测试的地位高于实现

- **MUST NOT** 为让新实现通过而修改既有断言、删用例、放宽期望值。先问「是测试错了还是实现错了」：
  实现错 → 改实现；行为确需变化 → 在 PR 写清旧行为 / 新行为 / 影响的调用方，再改测试，并按 `VERSIONING.md` 判断升版本。
- **MUST** characterization 用例（`httpx/characterization_test.go` 与 `make char` 覆盖的用例）的删除或断言变更，
  PR 标题或正文必须显式标注「行为变更」。
- **MUST** 修 bug 先写能复现它的失败测试，测试名写明锁的是哪个 bug 的行为。
