package geo

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode"

	"math/rand/v2"
)

func TestWebAcceptLanguageForCountry(t *testing.T) {
	cases := []struct {
		name       string
		country    string
		want       string
		wantErr    bool
		errCountry string
	}{
		{"印尼收成语种 id", "ID", "id", false, ""},
		{"美国保留 en-US", "US", "en-US", false, ""},
		{"英国保留 en-GB", "GB", "en-GB", false, ""},
		{"日本收成语种 ja", "JP", "ja", false, ""},
		{"巴西保留 pt-BR", "BR", "pt-BR", false, ""},
		{"中国保留 zh-CN", "CN", "zh-CN", false, ""},
		{"瑞士保留 de-CH", "CH", "de-CH", false, ""},
		{"阿联酋收成语种 ar", "AE", "ar", false, ""},
		{"表内非 Chrome 精确项仍原样返回", "ZW", "en-ZW", false, ""},
		{"小写国家码", "id", "id", false, ""},
		{"首尾空白", " jp ", "ja", false, ""},
		{"未知国家", "XX", "", true, "XX"},
		{"空串", "", "", true, ""},
		{"只空白", "  ", "", true, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := WebAcceptLanguageForCountry(c.country)
			t.Logf("country=%q → %q err=%v", c.country, got, err)
			if c.wantErr {
				if got != "" {
					t.Errorf("value = %q, want empty on error", got)
				}
				assertUnknownCountry(t, err, c.errCountry)
				return
			}
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestCountryToWebAcceptTag_满足构建前置条件(t *testing.T) {
	keys := make([]string, 0, len(countryToWebAcceptTag))
	for cc := range countryToWebAcceptTag {
		keys = append(keys, cc)
	}
	sort.Strings(keys)
	for _, cc := range keys {
		tag := countryToWebAcceptTag[cc]
		t.Logf("%s → %s", cc, tag)
		if tag == "" {
			t.Errorf("%s 的 Web 码为空", cc)
			continue
		}
		if strings.ContainsAny(tag, "_,; \t\n") {
			t.Errorf("%s 的 Web 码 %q 含 _ , ; 或空白，不能交给 BuildChromeAcceptLanguage", cc, tag)
		}
		for _, r := range tag {
			if r > unicode.MaxASCII {
				t.Errorf("%s 的 Web 码 %q 含非 ASCII", cc, tag)
				break
			}
		}
	}
}

func TestBuildChromeAcceptLanguage(t *testing.T) {
	cases := []struct {
		name string
		tags []string
		want string
	}{
		{"nil 返回空", nil, ""},
		{"空切片返回空", []string{}, ""},
		{"全空元素返回空", []string{"", "  "}, ""},
		{"截图五项已含中文", []string{"zh-CN", "zh", "en", "ja", "pt-BR"}, "zh-CN,zh;q=0.9,en;q=0.8,ja;q=0.7,pt-BR;q=0.6,pt;q=0.5"},
		{"自动补 zh 和 pt", []string{"zh-CN", "en", "ja", "pt-BR"}, "zh-CN,zh;q=0.9,en;q=0.8,ja;q=0.7,pt-BR;q=0.6,pt;q=0.5"},
		{"相邻同族 en-US en-GB 延后补 en", []string{"en-US", "en-GB", "fr"}, "en-US,en-GB;q=0.9,en;q=0.8,fr;q=0.7"},
		{"晚出现的 en 被前瞻去重提前", []string{"en-US", "fr", "en"}, "en-US,en;q=0.9,fr;q=0.8"},
		{"中文被 en 隔开时 zh 紧跟 zh-CN", []string{"zh-CN", "en", "ja", "pt-BR", "zh-HK"}, "zh-CN,zh;q=0.9,en;q=0.8,ja;q=0.7,pt-BR;q=0.6,pt;q=0.5,zh-HK;q=0.4"},
		{"相邻中文族末尾才补 zh", []string{"zh-CN", "zh-HK", "en", "ja", "pt-BR", "zh-TW"}, "zh-CN,zh-HK;q=0.9,zh;q=0.8,en;q=0.7,ja;q=0.6,pt-BR;q=0.5,pt;q=0.4,zh-TW;q=0.3"},
		{"完全重复 tag 去重", []string{"ja", "ja", "ko"}, "ja,ko;q=0.9"},
		{"私有标签 x 不补基础语言", []string{"x-pig-latin", "fr"}, "x-pig-latin,fr;q=0.9"},
		{"祖父标签 i 不补基础语言", []string{"i-klingon", "fr"}, "i-klingon,fr;q=0.9"},
		{"首尾空白 trim", []string{"  zh-CN  ", " en "}, "zh-CN,zh;q=0.9,en;q=0.8"},
		{"大小写精确去重不合并", []string{"zh-CN", "ZH-CN"}, "zh-CN,zh;q=0.9,ZH-CN;q=0.8,ZH;q=0.7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := BuildChromeAcceptLanguage(c.tags)
			t.Logf("tags=%q → %q", c.tags, got)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestBuildChromeAcceptLanguage_q值下限保持0点1(t *testing.T) {
	tags := []string{"ja", "ko", "th", "vi", "id", "ms", "nl", "sv", "da", "fi", "pl"}
	got := BuildChromeAcceptLanguage(tags)
	t.Logf("11 项 → %q", got)
	if strings.Contains(got, "q=0.0") {
		t.Fatalf("出现 q=0.0: %q", got)
	}
	want := "ja,ko;q=0.9,th;q=0.8,vi;q=0.7,id;q=0.6,ms;q=0.5,nl;q=0.4,sv;q=0.3,da;q=0.2,fi;q=0.1,pl;q=0.1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildChromeAcceptLanguageForCountry_只出本国(t *testing.T) {
	cases := []struct {
		country string
		want    string
	}{
		{"CN", "zh-CN,zh;q=0.9"},
		{"BR", "pt-BR,pt;q=0.9"},
		{"JP", "ja"},
		{"US", "en-US,en;q=0.9"},
	}
	for _, c := range cases {
		t.Run(c.country, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(1, 2))
			control := rand.New(rand.NewPCG(1, 2))
			got, err := BuildChromeAcceptLanguageForCountry(c.country, 0, rng)
			t.Logf("country=%q extra=0 → %q err=%v", c.country, got, err)
			if err != nil {
				t.Fatalf("unexpected err %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
			if rng.Uint64() != control.Uint64() {
				t.Fatal("extra=0 不得消费传入的 RNG")
			}
		})
	}
}

func TestBuildChromeAcceptLanguageForCountry_负数extra(t *testing.T) {
	cases := []struct {
		name    string
		country string
		extra   int
	}{
		{"普通负数", "CN", -1},
		{"更小的负数", "CN", -100},
		{"负数优先于未知国家", "XX", -1},
		{"负数优先于空国家", "", -2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rng := rand.New(rand.NewPCG(1, 2))
			control := rand.New(rand.NewPCG(1, 2))
			got, err := BuildChromeAcceptLanguageForCountry(c.country, c.extra, rng)
			t.Logf("country=%q extra=%d → %q err=%v", c.country, c.extra, got, err)
			if got != "" {
				t.Errorf("value = %q, want empty", got)
			}
			assertInvalidExtra(t, err, c.extra)
			if rng.Uint64() != control.Uint64() {
				t.Fatal("extra<0 不得消费传入的 RNG")
			}
		})
	}
}

func TestBuildChromeAcceptLanguageForCountry_未知国家(t *testing.T) {
	got, err := BuildChromeAcceptLanguageForCountry("XX", 0, nil)
	t.Logf("got=%q err=%v", got, err)
	if got != "" {
		t.Errorf("value = %q, want empty", got)
	}
	assertUnknownCountry(t, err, "XX")
}

func TestBuildChromeAcceptLanguageForCountry_extra超出候选池(t *testing.T) {
	available := remainingChromeExtraPool("zh-CN")
	extra := available + 1
	got, err := BuildChromeAcceptLanguageForCountry("CN", extra, nil)
	t.Logf("got=%q err=%v available=%d extra=%d", got, err, available, extra)
	if got != "" {
		t.Errorf("value = %q, want empty", got)
	}
	assertExtraExceedsPool(t, err, extra, available)
}

func TestBuildChromeAcceptLanguageForCountry_相同种子可复现(t *testing.T) {
	a, err := BuildChromeAcceptLanguageForCountry("CN", 3, rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildChromeAcceptLanguageForCountry("CN", 3, rand.New(rand.NewPCG(1, 2)))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seed(1,2) → %q", a)
	if a != b {
		t.Fatalf("相同 PCG seed 得到不同 header:\n%s\n%s", a, b)
	}
	if !strings.HasPrefix(a, "zh-CN") {
		t.Fatalf("必须以 zh-CN 开头, got %q", a)
	}
}

func TestSampleChromeLanguagePrefs_主码优先且顺序原样(t *testing.T) {
	const extra = 3
	rng := rand.New(rand.NewPCG(1, 2))
	got, err := sampleChromeLanguagePrefs("zh-CN", extra, rng)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("prefs=%q", got)
	if got[0] != "zh-CN" {
		t.Fatalf("主码应在第一项, got %q", got)
	}
	if len(got) != 1+extra {
		t.Fatalf("len=%d, want %d", len(got), 1+extra)
	}
	seen := map[string]struct{}{}
	for _, tag := range got {
		if _, ok := seen[tag]; ok {
			t.Fatalf("原始候选重复 %q", tag)
		}
		seen[tag] = struct{}{}
	}

	excluded := map[string]struct{}{}
	for _, tag := range expandChromeLanguageList([]string{"zh-CN"}) {
		excluded[tag] = struct{}{}
	}
	pool := uniqueSortedChromeTags()
	remain := make([]string, 0, len(pool))
	for _, tag := range pool {
		if _, skip := excluded[tag]; !skip {
			remain = append(remain, tag)
		}
	}
	perm := rand.New(rand.NewPCG(1, 2)).Perm(len(remain))
	want := []string{"zh-CN", remain[perm[0]], remain[perm[1]], remain[perm[2]]}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("抽样后不得二次排序或按语言族分组\ngot  %q\nwant %q", got, want)
	}
}

func TestBuildChromeAcceptLanguageForCountry_nilRNG结构(t *testing.T) {
	got, err := BuildChromeAcceptLanguageForCountry("CN", 2, nil)
	t.Logf("nil rng → %q err=%v", got, err)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "zh-CN") {
		t.Fatalf("必须以 zh-CN 开头, got %q", got)
	}
	if !strings.Contains(got, ",") {
		t.Fatalf("extra=2 应展开出多项, got %q", got)
	}
}

func TestBuildChromeAcceptLanguageForCountry_nilRNG可并发(t *testing.T) {
	var wg sync.WaitGroup
	errCh := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := BuildChromeAcceptLanguageForCountry("CN", 2, nil)
			if err != nil {
				errCh <- err
				return
			}
			if !strings.HasPrefix(h, "zh-CN") {
				errCh <- errors.New("header 不以 zh-CN 开头: " + h)
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func remainingChromeExtraPool(primary string) int {
	excluded := make(map[string]struct{})
	for _, tag := range expandChromeLanguageList([]string{primary}) {
		excluded[tag] = struct{}{}
	}
	n := 0
	for _, tag := range uniqueSortedChromeTags() {
		if _, skip := excluded[tag]; !skip {
			n++
		}
	}
	return n
}

func TestBuildChromeAcceptLanguage_不改调用方切片(t *testing.T) {
	in := []string{"  zh-CN  ", "en"}
	got := BuildChromeAcceptLanguage(in)
	t.Logf("in=%q → %q", in, got)
	if in[0] != "  zh-CN  " || in[1] != "en" {
		t.Fatalf("调用方切片被改写: %q", in)
	}
}

func TestBuildChromeAcceptLanguage_单项不写q(t *testing.T) {
	got := BuildChromeAcceptLanguage([]string{"ja"})
	t.Logf("tags=[ja] → %q", got)
	if got != "ja" {
		t.Errorf("got %q, want ja", got)
	}
}

func TestBuildChromeAcceptLanguageForCountry_extra等于候选池(t *testing.T) {
	available := remainingChromeExtraPool("ja")
	got, err := BuildChromeAcceptLanguageForCountry("JP", available, rand.New(rand.NewPCG(3, 4)))
	t.Logf("extra=%d → %q err=%v", available, got, err)
	if err != nil {
		t.Fatalf("extra==available 应成功: %v", err)
	}
	if !strings.HasPrefix(got, "ja") {
		t.Fatalf("必须以 ja 开头, got %q", got)
	}
}

func TestBuildChromeAcceptLanguageForCountry_extra为1(t *testing.T) {
	got, err := BuildChromeAcceptLanguageForCountry("JP", 1, rand.New(rand.NewPCG(5, 6)))
	t.Logf("JP extra=1 → %q err=%v", got, err)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "ja,") && got != "ja" {
		t.Fatalf("必须以 ja 开头, got %q", got)
	}
	if !strings.Contains(got, ",") {
		t.Fatalf("extra=1 应至少展开出两项, got %q", got)
	}
}

func TestChromeLanguageBase(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"无连字符原样", "ja", "ja"},
		{"地区码取首段", "zh-CN", "zh"},
		{"script 仍取首段", "zh-Hans-CN", "zh"},
		{"空串", "", ""},
		{"先导连字符不切", "-CN", "-CN"},
		{"尾部连字符仍取首段", "zh-", "zh"},
		{"私有标签", "x-pig-latin", "x"},
		{"祖父标签", "i-klingon", "i"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := chromeLanguageBase(c.in)
			t.Logf("%q → %q", c.in, got)
			if got != c.want {
				t.Errorf("chromeLanguageBase(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestExpandChromeLanguageList(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"nil", nil, []string{}},
		{"空切片", []string{}, []string{}},
		{"中间空元素丢掉", []string{"ja", "", "  ", "ko"}, []string{"ja", "ko"}},
		{"地区码补基础语言", []string{"zh-CN"}, []string{"zh-CN", "zh"}},
		{"相邻同族延后补", []string{"en-US", "en-GB"}, []string{"en-US", "en-GB", "en"}},
		{"已是基础语言不再重复补", []string{"ja"}, []string{"ja"}},
		{"x 不补", []string{"x-pig-latin"}, []string{"x-pig-latin"}},
		{"i 不补", []string{"i-klingon"}, []string{"i-klingon"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := expandChromeLanguageList(c.in)
			t.Logf("%q → %q", c.in, got)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestExpandChromeLanguageList_不改调用方切片(t *testing.T) {
	in := []string{"  zh-CN  ", "en"}
	_ = expandChromeLanguageList(in)
	t.Logf("after expand in=%q", in)
	if in[0] != "  zh-CN  " || in[1] != "en" {
		t.Fatalf("调用方切片被改写: %q", in)
	}
}

func TestFormatChromeAcceptLanguageQ(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want string
	}{
		{"空切片", []string{}, ""},
		{"nil", nil, ""},
		{"单项不写 q", []string{"ja"}, "ja"},
		{"两项第二项 q=0.9", []string{"zh-CN", "zh"}, "zh-CN,zh;q=0.9"},
		{"恰好 10 项末项 q=0.1", []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}, "a,b;q=0.9,c;q=0.8,d;q=0.7,e;q=0.6,f;q=0.5,g;q=0.4,h;q=0.3,i;q=0.2,j;q=0.1"},
		{"11 项末两项都是 q=0.1", []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"}, "a,b;q=0.9,c;q=0.8,d;q=0.7,e;q=0.6,f;q=0.5,g;q=0.4,h;q=0.3,i;q=0.2,j;q=0.1,k;q=0.1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatChromeAcceptLanguageQ(c.in)
			t.Logf("len=%d → %q", len(c.in), got)
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestUniqueSortedChromeTags_有序去重且副本独立(t *testing.T) {
	a := uniqueSortedChromeTags()
	t.Logf("unique=%d table=%d first=%q last=%q", len(a), len(countryToWebAcceptTag), a[0], a[len(a)-1])
	if len(a) == 0 {
		t.Fatal("候选池为空")
	}
	if len(a) >= len(countryToWebAcceptTag) {
		t.Fatal("表内有重复 data-code（如 MO/HK→zh-HK），去重后应短于国家数")
	}
	for i := 1; i < len(a); i++ {
		if a[i] <= a[i-1] {
			t.Fatalf("未严格升序: %q 后接 %q", a[i-1], a[i])
		}
	}
	orig0 := a[0]
	a[0] = "zzz"
	b := uniqueSortedChromeTags()
	t.Logf("改写副本后再次取出 first=%q", b[0])
	if b[0] != orig0 {
		t.Fatalf("第二次调用被上一次切片污染: %q", b[0])
	}
}

func TestSampleChromeLanguagePrefs_extra为0(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	control := rand.New(rand.NewPCG(1, 2))
	got, err := sampleChromeLanguagePrefs("ja", 0, rng)
	t.Logf("prefs=%q err=%v", got, err)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "ja" {
		t.Fatalf("got %q, want [ja]", got)
	}
	if rng.Uint64() != control.Uint64() {
		t.Fatal("extra=0 不得消费传入的 RNG")
	}
}

func TestSampleChromeLanguagePrefs_负数extra(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	control := rand.New(rand.NewPCG(1, 2))
	got, err := sampleChromeLanguagePrefs("zh-CN", -1, rng)
	t.Logf("prefs=%v err=%v", got, err)
	if got != nil {
		t.Errorf("prefs = %q, want nil", got)
	}
	assertInvalidExtra(t, err, -1)
	if rng.Uint64() != control.Uint64() {
		t.Fatal("extra<0 不得消费传入的 RNG")
	}
}

func TestSampleChromeLanguagePrefs_extra超出候选池(t *testing.T) {
	available := remainingChromeExtraPool("zh-CN")
	extra := available + 1
	got, err := sampleChromeLanguagePrefs("zh-CN", extra, nil)
	t.Logf("prefs=%v err=%v available=%d", got, err, available)
	if got != nil {
		t.Errorf("prefs = %q, want nil", got)
	}
	assertExtraExceedsPool(t, err, extra, available)
}

func TestSampleChromeLanguagePrefs_extra等于候选池(t *testing.T) {
	available := remainingChromeExtraPool("ja")
	got, err := sampleChromeLanguagePrefs("ja", available, rand.New(rand.NewPCG(7, 8)))
	t.Logf("len=%d available=%d err=%v", len(got), available, err)
	if err != nil {
		t.Fatalf("extra==available 应成功: %v", err)
	}
	if len(got) != 1+available {
		t.Fatalf("len=%d, want %d", len(got), 1+available)
	}
	if got[0] != "ja" {
		t.Fatalf("主码应在第一项, got %q", got[0])
	}
}

func TestSampleChromeLanguagePrefs_排除主码展开项(t *testing.T) {
	got, err := sampleChromeLanguagePrefs("zh-CN", 1, rand.New(rand.NewPCG(9, 10)))
	t.Logf("prefs=%q err=%v", got, err)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}
	excluded := map[string]struct{}{}
	for _, tag := range expandChromeLanguageList([]string{"zh-CN"}) {
		excluded[tag] = struct{}{}
	}
	if _, bad := excluded[got[1]]; bad {
		t.Fatalf("抽到主码展开项 %q", got[1])
	}
}
