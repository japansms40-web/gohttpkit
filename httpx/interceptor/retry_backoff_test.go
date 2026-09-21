package interceptor

import (
	"math"
	"testing"
	"time"
)

// retry_backoff_test.go —— computeBackoff 的边界/对抗测试。
// 契约（doc 承诺）：退避 = base*2^attempt，封顶 max；base>0 时结果必须 ∈ (0, max]（max>0），
// 绝不为 0 或负。指数退避在大 attempt 下不能整型溢出把 MaxBackoff 封顶绕过。

func TestComputeBackoff_正常翻倍(t *testing.T) {
	base := 200 * time.Millisecond
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 200 * time.Millisecond},
		{1, 400 * time.Millisecond},
		{2, 800 * time.Millisecond},
		{3, 1600 * time.Millisecond},
	}
	for _, c := range cases {
		got := computeBackoff(base, 0, c.attempt) // max=0 不封顶
		t.Logf("base=%v attempt=%d → %v (want %v)", base, c.attempt, got, c.want)
		if got != c.want {
			t.Errorf("attempt=%d：得到 %v，应为 %v", c.attempt, got, c.want)
		}
	}
}

func TestComputeBackoff_封顶(t *testing.T) {
	base := 1 * time.Second
	cases := []struct {
		max     time.Duration
		attempt int
		desc    string
	}{
		{5 * time.Second, 10, "循环内提前触顶：base*1024 远超 5s"},
		{3 * time.Second, 2, "最后一次翻倍才越界：1s→2s→4s，循环后钳到 3s"},
	}
	for _, c := range cases {
		got := computeBackoff(base, c.max, c.attempt)
		t.Logf("base=%v max=%v attempt=%d → %v（%s）", base, c.max, c.attempt, got, c.desc)
		if got != c.max {
			t.Errorf("%s：应封顶到 %v，得到 %v", c.desc, c.max, got)
		}
	}
}

// 核心 bug 用例：大 attempt 下 base*2^attempt 整型溢出，
// 退避不能变成 0 / 负数、也不能绕过 MaxBackoff。
func TestComputeBackoff_大attempt不溢出不绕过封顶(t *testing.T) {
	base, max := 1*time.Second, 5*time.Second
	for _, attempt := range []int{34, 40, 62, 63, 64, 100, 200} {
		got := computeBackoff(base, max, attempt)
		t.Logf("base=%v max=%v attempt=%d → %v", base, max, attempt, got)
		if got <= 0 {
			t.Errorf("attempt=%d：退避为 %v（≤0），指数退避整型溢出", attempt, got)
		}
		if got > max {
			t.Errorf("attempt=%d：退避 %v 超过封顶 %v，MaxBackoff 被绕过", attempt, got, max)
		}
	}
}

// 不封顶（max=0）时，大 attempt 也不能溢出成 0 / 负数——应饱和到一个正的大值。
func TestComputeBackoff_不封顶大attempt也不为负(t *testing.T) {
	base := 1 * time.Second
	for _, attempt := range []int{63, 64, 100, 200} {
		got := computeBackoff(base, 0, attempt)
		t.Logf("base=%v max=0 attempt=%d → %v", base, attempt, got)
		if got <= 0 {
			t.Errorf("attempt=%d：不封顶时退避为 %v（≤0），整型溢出", attempt, got)
		}
	}
}

// base<=0（防御，normalizeRetry 已保证不会发生）返回 0，不 panic。
func TestComputeBackoff_base非正返回0(t *testing.T) {
	for _, base := range []time.Duration{0, -1 * time.Second} {
		got := computeBackoff(base, 5*time.Second, 3)
		t.Logf("base=%v → %v", base, got)
		if got != 0 {
			t.Errorf("base=%v：应返回 0，得到 %v", base, got)
		}
	}
}

// 超大 max（接近 int64 上限）时，翻倍会先触发溢出保护而非 backoff>=max，结果仍 ∈ (0, max]。
func TestComputeBackoff_超大max溢出前钳住(t *testing.T) {
	base := 1 * time.Second
	max := time.Duration(math.MaxInt64)
	got := computeBackoff(base, max, 200)
	t.Logf("base=%v max=%v attempt=200 → %v", base, max, got)
	if got <= 0 || got > max {
		t.Errorf("应 ∈ (0, %v]，得到 %v", max, got)
	}
}
