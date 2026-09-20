package geo

import "testing"

func TestNormalizeCountryKey_trim并大写(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"已是大写两位", "ID", "ID"},
		{"小写", "id", "ID"},
		{"大小写混用", "Id", "ID"},
		{"首尾空白", " jp ", "JP"},
		{"制表与换行", "\tjp\n", "JP"},
		{"空串", "", ""},
		{"只空白", "  ", ""},
		{"三位不去校验合法性", "usa", "USA"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeCountryKey(c.in)
			t.Logf("%q → %q", c.in, got)
			if got != c.want {
				t.Errorf("normalizeCountryKey(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestLookupCountry_空输入报空码错误(t *testing.T) {
	table := map[string]string{"ID": "id"}
	cases := []struct {
		name string
		in   string
	}{
		{"空串", ""},
		{"只空白", "  "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := lookupCountry(table, c.in)
			t.Logf("in=%q → %q err=%v", c.in, got, err)
			if got != "" {
				t.Errorf("value = %q, want empty", got)
			}
			assertUnknownCountry(t, err, "")
		})
	}
}

func TestLookupCountry_未命中报归一化码(t *testing.T) {
	got, err := lookupCountry(map[string]string{"ID": "id"}, "xx")
	t.Logf("in=%q → %q err=%v", "xx", got, err)
	if got != "" {
		t.Errorf("value = %q, want empty", got)
	}
	assertUnknownCountry(t, err, "XX")
}

func TestLookupCountry_命中返回表值(t *testing.T) {
	got, err := lookupCountry(map[string]string{"ID": "id"}, " id ")
	t.Logf("in=%q → %q err=%v", " id ", got, err)
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if got != "id" {
		t.Errorf("got %q, want id", got)
	}
}

func TestLookupCountry_nil表当未命中(t *testing.T) {
	got, err := lookupCountry(nil, "ID")
	t.Logf("nil table → %q err=%v", got, err)
	if got != "" {
		t.Errorf("value = %q, want empty", got)
	}
	assertUnknownCountry(t, err, "ID")
}

func TestLookupCountry_空表当未命中(t *testing.T) {
	got, err := lookupCountry(map[string]string{}, "ID")
	t.Logf("empty table → %q err=%v", got, err)
	if got != "" {
		t.Errorf("value = %q, want empty", got)
	}
	assertUnknownCountry(t, err, "ID")
}

func TestLookupCountry_表值为空串仍是命中(t *testing.T) {
	got, err := lookupCountry(map[string]string{"ID": ""}, "id")
	t.Logf("empty value → %q err=%v", got, err)
	if err != nil {
		t.Fatalf("空串表值是命中不是错误: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string value", got)
	}
}

func TestLookupCountry_不改调用方表(t *testing.T) {
	table := map[string]string{"ID": "id"}
	_, _ = lookupCountry(table, "xx")
	t.Logf("after miss table=%v", table)
	if _, ok := table["XX"]; ok {
		t.Fatal("未命中不应往调用方表里写 key")
	}
	if table["ID"] != "id" || len(table) != 1 {
		t.Fatalf("调用方表被改写: %v", table)
	}
}
