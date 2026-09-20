package versionreg_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/versionreg"
)

func TestConfig_未配置项返回零值(t *testing.T) {
	cfg := versionreg.NewConfig("v1", "https://a.example")
	t.Logf("DocID=%q Param=%q WL=%v", cfg.DocID("nope"), cfg.Param("nope"), cfg.HeaderWhitelist("nope"))
	if cfg.DocID("nope") != "" {
		t.Fatal("未配置 docIDs 时应返回空串")
	}
	if cfg.Param("nope") != "" {
		t.Fatal("未配置 Params 时应返回空串")
	}
	if cfg.HeaderWhitelist("nope") != nil {
		t.Fatal("未配置白名单时应返回 nil(= 发全量头)")
	}
}

func TestConfig_Validate缺ID(t *testing.T) {
	cfg := versionreg.NewConfig("", "https://a.example")
	err := cfg.Validate()
	t.Logf("缺 ID → %v", err)
	assertMissingField(t, err, "ID")
}

func TestConfig_Option可叠加(t *testing.T) {
	cfg := versionreg.NewConfig("v1", "https://a.example",
		versionreg.WithParams(map[string]string{"a": "1"}),
		versionreg.WithParams(map[string]string{"b": "2"}),
		versionreg.WithDocIDs(map[versionreg.Endpoint]string{"e1": "d1"}),
		versionreg.WithDocIDs(map[versionreg.Endpoint]string{"e2": "d2"}),
	)
	t.Logf("Params=%v e1=%q e2=%q", cfg.Params, cfg.DocID("e1"), cfg.DocID("e2"))
	if cfg.Param("a") != "1" || cfg.Param("b") != "2" {
		t.Fatalf("WithParams 应合并, got %v", cfg.Params)
	}
	if cfg.DocID("e1") != "d1" || cfg.DocID("e2") != "d2" {
		t.Fatal("WithDocIDs 应合并")
	}
}

func TestConfig_WithParams_nil不增加项(t *testing.T) {
	cfg := versionreg.NewConfig("v1", "https://a.example", versionreg.WithParams(nil))
	t.Logf("WithParams(nil) Params=%v Param(x)=%q", cfg.Params, cfg.Param("x"))
	if cfg.Param("x") != "" {
		t.Fatal("WithParams(nil) 访问仍应返回空串")
	}
	if len(cfg.Params) != 0 {
		t.Fatalf("WithParams(nil) 不应增加参数, got %v", cfg.Params)
	}
}
