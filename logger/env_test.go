package logger

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/internal/envx"
)

// TestEnvConfig 表驱动验证 envConfig 对各环境变量组合的解析
// 角度: #1 边界 + #6 契约不变式 —— 无 env 不偏离默认;FILE 推导 both;OUTPUT 显式覆盖
func TestEnvConfig(t *testing.T) {
	cases := []struct {
		name       string
		env        map[string]string
		wantOK     bool
		wantLevel  string
		wantFormat string
		wantOutput string
		wantFile   string
	}{
		{
			name:   "无任何 env 保持默认零惊扰",
			env:    map[string]string{},
			wantOK: false,
		},
		{
			name:       "仅 FILE 推导 both 落盘",
			env:        map[string]string{envx.Key(envLogFile): "/tmp/a.log"},
			wantOK:     true,
			wantLevel:  "info",
			wantFormat: "json",
			wantOutput: "both",
			wantFile:   "/tmp/a.log",
		},
		{
			name:       "OUTPUT 显式覆盖 FILE 推导的 both",
			env:        map[string]string{envx.Key(envLogFile): "/tmp/a.log", envx.Key(envLogOutput): "file"},
			wantOK:     true,
			wantLevel:  "info",
			wantFormat: "json",
			wantOutput: "file",
			wantFile:   "/tmp/a.log",
		},
		{
			name:       "仅 LEVEL 偏离默认其余不变",
			env:        map[string]string{envx.Key(envLogLevel): "debug"},
			wantOK:     true,
			wantLevel:  "debug",
			wantFormat: "json",
			wantOutput: "console",
		},
		{
			name:       "FORMAT 大小写归一",
			env:        map[string]string{envx.Key(envLogFormat): "Console"},
			wantOK:     true,
			wantLevel:  "info",
			wantFormat: "console",
			wantOutput: "console",
		},
		{
			name:   "空白被 TrimSpace 视为未设置",
			env:    map[string]string{envx.Key(envLogFile): "   "},
			wantOK: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 清空全部相关变量,再按用例设置(t.Setenv 测试结束自动还原)
			for _, k := range []string{envx.Key(envLogFile), envx.Key(envLogLevel), envx.Key(envLogFormat), envx.Key(envLogOutput)} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}

			c, ok := envConfig()
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (cfg=%+v)", ok, tc.wantOK, c)
			}
			if !tc.wantOK {
				return
			}
			if c.Level != tc.wantLevel {
				t.Errorf("Level = %q, want %q", c.Level, tc.wantLevel)
			}
			if c.Format != tc.wantFormat {
				t.Errorf("Format = %q, want %q", c.Format, tc.wantFormat)
			}
			if c.Output != tc.wantOutput {
				t.Errorf("Output = %q, want %q", c.Output, tc.wantOutput)
			}
			if c.FilePath != tc.wantFile {
				t.Errorf("FilePath = %q, want %q", c.FilePath, tc.wantFile)
			}
		})
	}
}

func TestApplyEnv_无相关变量时返回false(t *testing.T) {
	for _, k := range []string{envx.Key(envLogFile), envx.Key(envLogLevel), envx.Key(envLogFormat), envx.Key(envLogOutput)} {
		t.Setenv(k, "")
	}
	if ApplyEnv() {
		t.Fatal("无任何相关变量时应返回 false，且不覆盖已有配置")
	}
}

func TestApplyEnv_换前缀后重新生效(t *testing.T) {
	old := envx.Prefix()
	envx.SetPrefix("OTHER_")
	t.Cleanup(func() { envx.SetPrefix(old) })

	t.Setenv("OTHER_LOG_LEVEL", "warn")
	if !ApplyEnv() {
		t.Fatal("换前缀后 ApplyEnv 应读到配置")
	}
}
