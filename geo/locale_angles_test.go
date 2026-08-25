package geo

import (
	"encoding/json"
	"strings"
	"testing"
)

// 本文件用「多角度测试覆盖」方法论补全 internal/ 下 locale/timezone 相关函数的测试。
// 所有顶层标识符以 LcA 前缀，仅写入本文件，不改动生产代码与既有测试。
//
// 覆盖角度（per test-coverage-angles）：
//   #1 happy path / #2 boundaries / #5 nil-zero / #7 contract invariants
//   #11 input diversity（表驱动）/ #12 adversarial/malformed
// 通过 reverse-reasoning 定位每个未覆盖分支的触发输入：
//   formatWebAcceptLanguage  : 裸 "en"（primary==web）→ "en;q=0.9"
//   looksLikeBCP47           : len<2 的 false 分支；数字 case 标签
//   Android*（Mapped/Accept/Scripted/DeviceLanguagesJSON）：仅接收已派生的非空
//     locale（空串默认已删，由 NewClient 构造校验保证非空），覆盖归一化/脚本/去重分支

// ---------------------------------------------------------------------------
// formatWebAcceptLanguage
// ---------------------------------------------------------------------------

// LcATestFormatWebAcceptLanguage 直接驱动 formatWebAcceptLanguage 的全部三个分支。
// 关键缺口：裸 "en"（无 region，primary==web=="en"）只在显式 locale "en" 路径出现，
// 国家码路径永远带 region（en-US 等），所以表驱动测试漏掉了 line 184。
func TestLcAFormatWebAcceptLanguageAngles(t *testing.T) {
	cases := []struct {
		name string
		web  string
		want string
	}{
		// #2 boundary：裸 "en"（无连字符）——primary==web=="en"，跳 en-US 兜底且不重复
		{"bare en (no region)", "en", "en;q=0.9"},
		// #1 happy：en-XX（en 主语言带 region）不叠 en-US 兜底
		{"en with region", "en-US", "en-US,en;q=0.9"},
		{"en-GB", "en-GB", "en-GB,en;q=0.9"},
		// #1 happy：非 en 主语言走三段
		{"non-en id", "id-ID", "id-ID,id;q=0.9,en-US;q=0.8,en;q=0.7"},
		// #11 input diversity：裸非 en 主语言（无 region）——primary==web!=en，仍三段
		{"bare non-en fr", "fr", "fr,fr;q=0.9,en-US;q=0.8,en;q=0.7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatWebAcceptLanguage(c.web); got != c.want {
				t.Errorf("formatWebAcceptLanguage(%q) = %q, want %q", c.web, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// looksLikeBCP47
// ---------------------------------------------------------------------------

// LcATestLooksLikeBCP47Angles 覆盖 looksLikeBCP47 的判定边界与字符分类全分支。
// 缺口：localeToWebAcceptLanguage 对 "" 提前 return，使 looksLikeBCP47 从未收到
// len<2 输入（line 195 false 分支），数字 case 标签（line 202）也从未触发。
// 该函数不导出，但本测试在同包 internal 内可直接调用。
func TestLcALooksLikeBCP47Angles(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		// #2 boundary：len<2 一律 false（覆盖 line 195）
		{"empty", "", false},
		{"single letter", "x", false},
		{"single digit", "1", false},
		{"single hyphen", "-", false},
		// #2 boundary：恰好 2 字符且合法 → true
		{"two letters", "en", true},
		// #11 diversity：合法 BCP47 含连字符
		{"lang-region", "id-ID", true},
		// #12/#11：含数字（覆盖 line 202 数字 case 标签）——es-419 拉美西语
		{"with digits es-419", "es-419", true},
		{"pure digits len>=2", "12", true},
		{"mixed alnum hyphen", "zh-Hans-CN", true},
		// 大小写都接受
		{"upper", "EN-US", true},
		{"mixed case", "Zh-Hant", true},
		// #12 adversarial/malformed：非字母数字连字符 → false（default 分支）
		{"bang", "!!", false},
		{"underscore", "id_ID", false},
		{"space inside", "id ID", false},
		{"unicode", "中文", false},
		{"injection", "en\"><script>", false},
		// #12 oversized：超长仍按字符逐位判定（全合法 → true）
		{"oversized valid", strings.Repeat("a", 5000), true},
		// #12 oversized 含非法尾字符 → false
		{"oversized with bad tail", strings.Repeat("a", 5000) + "!", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := looksLikeBCP47(c.in); got != c.want {
				t.Errorf("looksLikeBCP47(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// LcATestLooksLikeBCP47ContractWiredIntoLocaleToWeb 契约不变量(#7)：
// looksLikeBCP47 的判定真实决定 localeToWebAcceptLanguage 是否返回空。
// 单字符/非法字符输入必须使 localeToWebAcceptLanguage 返回 ""（脏输入门控）。
func TestLcALooksLikeBCP47ContractWiredIntoLocaleToWeb(t *testing.T) {
	// 单字符："x" → looksLikeBCP47 false → 返回 ""
	if got := localeToWebAcceptLanguage("x"); got != "" {
		t.Errorf("localeToWebAcceptLanguage(%q) = %q, want \"\"（单字符应被 looksLikeBCP47 拒绝）", "x", got)
	}
	// 含数字合法："es-419" → looksLikeBCP47 true → 正常派生（非空）
	if got := localeToWebAcceptLanguage("es-419"); got == "" {
		t.Errorf("localeToWebAcceptLanguage(%q) 不应为空（含数字仍是合法 BCP47）", "es-419")
	}
	// 非法字符："id ID"（空格）→ looksLikeBCP47 false → 返回 ""
	if got := localeToWebAcceptLanguage("id ID"); got != "" {
		t.Errorf("localeToWebAcceptLanguage(%q) = %q, want \"\"（空格非法）", "id ID", got)
	}
}

// ---------------------------------------------------------------------------
// AndroidMappedLocale
// ---------------------------------------------------------------------------

// LcATestAndroidMappedLocaleAngles 覆盖 AndroidMappedLocale 空串兜底与归一化分支。
func TestLcAAndroidMappedLocaleAngles(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// #1 happy：连字符归一化为下划线
		{"hyphen to underscore", "zh-CN", "zh_CN"},
		// #11 diversity：已是下划线则原样
		{"already underscore", "es_UY", "es_UY"},
		{"en_US", "en_US", "en_US"},
		// #12 malformed：trim 周边空白
		{"trim whitespace", "  pt-BR  ", "pt_BR"},
		// #2 boundary：纯连字符全替换
		{"all hyphens", "a-b-c", "a_b_c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AndroidMappedLocale(c.in); got != c.want {
				t.Errorf("AndroidMappedLocale(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AndroidAcceptLanguage
// ---------------------------------------------------------------------------

// LcATestAndroidAcceptLanguageAngles 覆盖 AndroidAcceptLanguage 两个分支：
// en 主语言只回 "en-US"、非 en 加英语备选。
func TestLcAAndroidAcceptLanguageAngles(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// #1 happy：en 主语言去重，只回 en-US
		{"en_US dedup", "en_US", "en-US"},
		{"en-GB dedup", "en-GB", "en-US"},
		// #7 contract：主语言判定大小写不敏感（EqualFold）——"EN_us" 仍判为 en
		{"uppercase EN", "EN_us", "en-US"},
		// #1 happy：非 en 主语言加 en-US 备选，下划线转连字符
		{"non-en zh", "zh_CN", "zh-CN, en-US"},
		{"non-en es", "es_UY", "es-UY, en-US"},
		// #11 diversity：已是连字符输入
		{"hyphen input", "id-ID", "id-ID, en-US"},
		// #12 malformed：trim
		{"trim", "  pt_BR  ", "pt-BR, en-US"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AndroidAcceptLanguage(c.in); got != c.want {
				t.Errorf("AndroidAcceptLanguage(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AndroidScriptedLocale
// ---------------------------------------------------------------------------

// LcATestAndroidScriptedLocaleAngles 覆盖 AndroidScriptedLocale 两分支：
// 命中脚本白名单加 _#Script、未命中原样。
func TestLcAAndroidScriptedLocaleAngles(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// #1 happy：简体中文白名单加 Hans
		{"zh_CN Hans", "zh_CN", "zh_CN_#Hans"},
		{"zh_SG Hans", "zh_SG", "zh_SG_#Hans"},
		// #1 happy：繁体中文白名单加 Hant
		{"zh_TW Hant", "zh_TW", "zh_TW_#Hant"},
		{"zh_HK Hant", "zh_HK", "zh_HK_#Hant"},
		// #7 contract：连字符输入先归一化为下划线再查白名单（zh-CN 必须命中）
		{"hyphen zh-CN normalized", "zh-CN", "zh_CN_#Hans"},
		// #11 diversity：白名单外原样返回（无脚本后缀）
		{"non-script en_IN", "en_IN", "en_IN"},
		{"non-script pt-BR normalized", "pt-BR", "pt_BR"},
		// #12 malformed：trim
		{"trim", "  zh_TW  ", "zh_TW_#Hant"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AndroidScriptedLocale(c.in); got != c.want {
				t.Errorf("AndroidScriptedLocale(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AndroidDeviceLanguagesJSON
// ---------------------------------------------------------------------------

// LcATestAndroidDeviceLanguagesJSONAngles 覆盖 en/非 en 两分支，
// 并断言精确字符串（key 顺序敏感，生产代码刻意手拼 JSON）。
func TestLcAAndroidDeviceLanguagesJSONAngles(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// #1 happy：非 en 主语言，system_languages 加 en-US 备选
		{"non-en es_UY", "es_UY", `{"system_languages":"es-UY, en-US","keyboard_language":"en-US"}`},
		// #1 happy：en 主语言不重复 en-US（EqualFold 判定）
		{"en_US no dup", "en_US", `{"system_languages":"en-US","keyboard_language":"en-US"}`},
		{"upper EN dedup", "EN_gb", `{"system_languages":"EN-gb","keyboard_language":"en-US"}`},
		// #11 diversity：连字符输入归一
		{"hyphen id-ID", "id-ID", `{"system_languages":"id-ID, en-US","keyboard_language":"en-US"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AndroidDeviceLanguagesJSON(c.in); got != c.want {
				t.Errorf("AndroidDeviceLanguagesJSON(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// LcATestAndroidDeviceLanguagesJSONIsValidJSON 契约不变量(#7)：
// 无论输入如何，输出必须是合法 JSON，且结构稳定（恰两个 string 字段，
// keyboard_language 恒为 en-US，system_languages 非空且含主语言）。
// reverse-reasoning：手拼 JSON 最大风险是特殊字符破坏合法性，这里穷举多种 locale 验证。
func TestLcAAndroidDeviceLanguagesJSONIsValidJSON(t *testing.T) {
	inputs := []string{"zh_CN", "es_UY", "en_US", "id-ID", "pt_BR", "ar_SA", "ja_JP", "  fr-FR  "}
	for _, in := range inputs {
		t.Run("input="+in, func(t *testing.T) {
			out := AndroidDeviceLanguagesJSON(in)

			// 必须解析为合法 JSON 对象
			var m map[string]any
			if err := json.Unmarshal([]byte(out), &m); err != nil {
				t.Fatalf("AndroidDeviceLanguagesJSON(%q) = %q 非合法 JSON: %v", in, out, err)
			}
			// 结构稳定：恰好两个字段
			if len(m) != 2 {
				t.Errorf("字段数 = %d, want 2；输出 %q", len(m), out)
			}
			// keyboard_language 恒为 en-US
			if kb, ok := m["keyboard_language"].(string); !ok || kb != "en-US" {
				t.Errorf("keyboard_language = %v, want \"en-US\"", m["keyboard_language"])
			}
			// system_languages 必须是非空字符串
			sys, ok := m["system_languages"].(string)
			if !ok || sys == "" {
				t.Errorf("system_languages 缺失或为空：%v", m["system_languages"])
			}
			// key 顺序敏感：system_languages 必须在 keyboard_language 之前（生产注释明确要求）
			si := strings.Index(out, `"system_languages"`)
			ki := strings.Index(out, `"keyboard_language"`)
			if si < 0 || ki < 0 || si >= ki {
				t.Errorf("key 顺序错误，system_languages 必须在 keyboard_language 之前：%q", out)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// primaryLanguage（共用主语言提取，间接影响上面所有派生）
// ---------------------------------------------------------------------------

// LcATestPrimaryLanguageAngles 守护主语言提取边界：有/无连字符、前导连字符。
func TestLcAPrimaryLanguageAngles(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// #1 happy：取第一段
		{"id-ID", "id"},
		{"zh-Hans-CN", "zh"},
		// #2 boundary：无连字符 → 原样
		{"en", "en"},
		{"", ""},
		// #2 boundary：前导连字符（idx==0，不满足 idx>0）→ 原样返回整串
		{"-US", "-US"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := primaryLanguage(c.in); got != c.want {
				t.Errorf("primaryLanguage(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
