package versionreg_test

// example_config_more_test.go —— Config / Option / NewConfig 的角度测试。
// 现有测试已锁未配置零值、缺 ID、Option 叠加、WithParams(nil)。
// 这里补 Validate 短路顺序、空白不算空、后写覆盖、白名单替换不合并、副本隔离。
// out-of-scope：nil *Config 接收者（文档未承诺 nil 安全，与 HeaderWhitelists 不同）。

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

func TestConfig_VersionID空与空白(t *testing.T) {
	cases := []versionreg.ID{"", " ", "v1", "  v1"}
	for _, id := range cases {
		cfg := versionreg.NewConfig(id, moreBaseURL)
		got := cfg.VersionID()
		t.Logf("NewConfig(%q).VersionID() = %q", string(id), got)
		if got != id {
			t.Errorf("VersionID() = %q, want %q（原样返回 c.ID）", got, id)
		}
	}
}

func TestConfig_Validate通过(t *testing.T) {
	cfg := versionreg.NewConfig("v1", moreBaseURL)
	err := cfg.Validate()
	t.Logf("齐全字段 Validate → %v", err)
	if err != nil {
		t.Fatalf("齐全字段应通过，得到 %v", err)
	}
}

func TestConfig_Validate两者都缺先报ID(t *testing.T) {
	err := versionreg.NewConfig("", "").Validate()
	t.Logf("ID 与 BaseURL 都空 → %v", err)
	assertMissingField(t, err, "ID")
}

func TestConfig_Validate空白不算空(t *testing.T) {
	err := versionreg.NewConfig("  ", moreBaseURL).Validate()
	t.Logf("空白 ID → %v", err)
	if err != nil {
		t.Fatalf("空白 ID 不是空串，Validate 应通过，得到 %v", err)
	}
	err = versionreg.NewConfig("v1", "  ").Validate()
	t.Logf("空白 BaseURL → %v", err)
	if err != nil {
		t.Fatalf("空白 BaseURL 不是空串，Validate 应通过，得到 %v", err)
	}
}

func TestConfig_HeaderWhitelist副本隔离(t *testing.T) {
	cfg := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
		"user.profile": {"accept": "", "x-app-id": "123"},
	}))
	wl := cfg.HeaderWhitelist("user.profile")
	wl["injected"] = "boom"
	again := cfg.HeaderWhitelist("user.profile")
	t.Logf("改副本后再次 HeaderWhitelist=%v", again)
	if _, leaked := again["injected"]; leaked {
		t.Fatal("HeaderWhitelist 必须返回副本")
	}
	if cfg.HeaderWhitelist("zzz") != nil {
		t.Fatal("未配置 endpoint 应为 nil")
	}
}

func TestConfig_DocID已配置与空值与未命中(t *testing.T) {
	cfg := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithDocIDs(map[versionreg.Endpoint]string{
		"user.profile": "doc-9",
		"empty":        "",
	}))
	cases := []struct {
		ep   versionreg.Endpoint
		want string
	}{
		{"user.profile", "doc-9"},
		{"empty", ""},
		{"zzz", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := cfg.DocID(c.ep)
		t.Logf("DocID(%q) = %q", string(c.ep), got)
		if got != c.want {
			t.Errorf("DocID(%q) = %q, want %q", string(c.ep), got, c.want)
		}
	}
}

func TestConfig_Param空键空值与未命中(t *testing.T) {
	cfg := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithParams(map[string]string{
		"app_id": "123",
		"":       "empty-key",
		"blank":  "",
	}))
	cases := []struct {
		key  string
		want string
	}{
		{"app_id", "123"},
		{"", "empty-key"},
		{"blank", ""},
		{"nope", ""},
	}
	for _, c := range cases {
		got := cfg.Param(c.key)
		t.Logf("Param(%q) = %q", c.key, got)
		if got != c.want {
			t.Errorf("Param(%q) = %q, want %q", c.key, got, c.want)
		}
	}
}

func TestWithUserAgent_空串与后写覆盖(t *testing.T) {
	empty := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithUserAgent(""))
	t.Logf("WithUserAgent(\"\") = %q", empty.UserAgent)
	if empty.UserAgent != "" {
		t.Fatalf("空 UA 应原样写入，得到 %q", empty.UserAgent)
	}

	cfg := versionreg.NewConfig("v1", moreBaseURL,
		versionreg.WithUserAgent("ua/1"),
		versionreg.WithUserAgent("ua/2"),
	)
	t.Logf("后写 UA = %q", cfg.UserAgent)
	if cfg.UserAgent != "ua/2" {
		t.Fatalf("后写应覆盖，得到 %q", cfg.UserAgent)
	}
}

func TestWithParams_同名覆盖与空map(t *testing.T) {
	cfg := versionreg.NewConfig("v1", moreBaseURL,
		versionreg.WithParams(map[string]string{"a": "1", "keep": "x"}),
		versionreg.WithParams(map[string]string{"a": "2"}),
	)
	t.Logf("覆盖后 Params=%v", cfg.Params)
	if cfg.Param("a") != "2" {
		t.Fatalf("同名 key 应被后写覆盖，得到 %q", cfg.Param("a"))
	}
	if cfg.Param("keep") != "x" {
		t.Fatal("未出现在后写里的 key 应保留")
	}

	empty := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithParams(map[string]string{}))
	t.Logf("空 map Params=%v Param(x)=%q", empty.Params, empty.Param("x"))
	if empty.Param("x") != "" {
		t.Fatal("空 map 访问仍应返回空串")
	}
	if len(empty.Params) != 0 {
		t.Fatalf("空 map 不应增加参数, got %v", empty.Params)
	}
}

func TestWithDocIDs_nil与同名覆盖(t *testing.T) {
	nilCfg := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithDocIDs(nil))
	t.Logf("WithDocIDs(nil) DocID(x)=%q", nilCfg.DocID("x"))
	if nilCfg.DocID("x") != "" {
		t.Fatal("WithDocIDs(nil) 访问仍应返回空串")
	}

	cfg := versionreg.NewConfig("v1", moreBaseURL,
		versionreg.WithDocIDs(map[versionreg.Endpoint]string{"e1": "d1", "keep": "k"}),
		versionreg.WithDocIDs(map[versionreg.Endpoint]string{"e1": "d2"}),
	)
	t.Logf("e1=%q keep=%q", cfg.DocID("e1"), cfg.DocID("keep"))
	if cfg.DocID("e1") != "d2" {
		t.Fatalf("同名 endpoint 应被后写覆盖，得到 %q", cfg.DocID("e1"))
	}
	if cfg.DocID("keep") != "k" {
		t.Fatal("未出现在后写里的 endpoint 应保留")
	}
}

func TestWithHeaderWhitelists_后写替换不合并(t *testing.T) {
	cfg := versionreg.NewConfig("v1", moreBaseURL,
		versionreg.WithHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
			"a": {"x": "1"},
		}),
		versionreg.WithHeaderWhitelists(map[versionreg.Endpoint]map[string]string{
			"b": {"y": "2"},
		}),
	)
	t.Logf("a=%v b=%v", cfg.HeaderWhitelist("a"), cfg.HeaderWhitelist("b"))
	if cfg.HeaderWhitelist("a") != nil {
		t.Fatal("WithHeaderWhitelists 是整表替换，不是按 endpoint 合并")
	}
	if got := cfg.HeaderWhitelist("b"); got["y"] != "2" {
		t.Fatalf("后写表应生效，b=%v", got)
	}
}

func TestWithHeaderWhitelists_nil是空集合不是未设置(t *testing.T) {
	cfg := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithHeaderWhitelists(nil))
	got := cfg.HeaderWhitelist("x")
	t.Logf("WithHeaderWhitelists(nil) HeaderWhitelist=%v", got)
	if got != nil {
		t.Fatal("空集合 For 仍为 nil（发全量头），与未配置观察值相同")
	}
}

func TestWithHeaderWhitelists_深拷贝入参(t *testing.T) {
	src := map[versionreg.Endpoint]map[string]string{
		"user.profile": {"accept": ""},
	}
	cfg := versionreg.NewConfig("v1", moreBaseURL, versionreg.WithHeaderWhitelists(src))
	src["user.profile"]["injected"] = "boom"
	got := cfg.HeaderWhitelist("user.profile")
	t.Logf("改入参后 HeaderWhitelist=%v", got)
	if _, leaked := got["injected"]; leaked {
		t.Fatal("WithHeaderWhitelists 必须深拷贝入参")
	}
}

func TestNewConfig_无Option与不在构造时Validate(t *testing.T) {
	cfg := versionreg.NewConfig("", "")
	t.Logf("无 Option ID=%q BaseURL=%q UA=%q Params=%v WL=%v Doc=%q",
		cfg.ID, cfg.BaseURL, cfg.UserAgent, cfg.Params, cfg.HeaderWhitelist("x"), cfg.DocID("x"))
	if cfg.ID != "" || cfg.BaseURL != "" {
		t.Fatal("空必填字段应原样写入，不在 NewConfig 里 Validate")
	}
	if cfg.UserAgent != "" || cfg.Params != nil {
		t.Fatal("无 Option 时可选字段应保持零值")
	}
	if cfg.HeaderWhitelist("x") != nil || cfg.DocID("x") != "" || cfg.Param("x") != "" {
		t.Fatal("无 Option 时访问器应返回零值")
	}
	assertMissingField(t, cfg.Validate(), "ID")
}

func TestNewConfig_Option按顺序叠加最后覆盖UA(t *testing.T) {
	cfg := versionreg.NewConfig("v1", moreBaseURL,
		versionreg.WithUserAgent("first"),
		versionreg.WithParams(map[string]string{"k": "1"}),
		versionreg.WithUserAgent("last"),
	)
	t.Logf("UA=%q k=%q", cfg.UserAgent, cfg.Param("k"))
	if cfg.UserAgent != "last" {
		t.Fatalf("后写 Option 应覆盖 UA，得到 %q", cfg.UserAgent)
	}
	if cfg.Param("k") != "1" {
		t.Fatal("中间的 WithParams 应已生效")
	}
}
