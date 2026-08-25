package geo

import "testing"

func TestParseCountryFromProxyURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"rola 实际样本 country-us", "socks5://new001kOtD8x_559143-sesstime-20-country-us:Jz2EiIyN6s4H@gate5.rola.vip:2031", "US"},
		{"rola 实际样本 country-kr", "socks5://new001kOtD8x_752086-sesstime-20-country-kr:Jz2EiIyN6s4H@gate5.rola.vip:2031", "KR"},
		{"大写 country-US 也接受", "socks5://user-country-US:pwd@host:1080", "US"},
		{"http proxy 同样支持", "http://u_country-uy-extra:p@gate.example.com:8080", "UY"},
		{"socks5h 同样支持", "socks5h://u-country-pr:p@h:1", "PR"},
		{"无 country 段返回空", "socks5://new001_sesstime-20:pwd@proxysg.rola.vip:2000", ""},
		{"完全无 user 段返回空", "socks5://gate.example.com:1080", ""},
		{"空串返回空", "", ""},
		{"畸形 URL 返回空（不 panic）", "://not a url", ""},
		{"country-united 这种长串不误命中", "socks5://u-country-united-states:p@h:1", ""},
		{"country-zz 不存在的 ISO 也照常返回（由派生层判断是否查得到）", "socks5://u-country-zz:p@h:1", "ZZ"},

		// 密码段：evomi 这类代理要求 country/session 写在密码里而不是用户名里
		// （InsExecutor pkg/proxy/url_builder.go 的 Password 模板就是为它加的）。
		// 只扫用户名会让整条画像派生链静默落空——2026-08-17 生产事故：出口 us
		// 却因为 Country="" 兜底成 zh-CN，服务端按中文 locale 渲染并弹「根据你所在
		// 地区出台的新法律规定，无法在这里创建账户」。
		{"evomi 实际样本 country 在密码段", "socks5://insxieyitu4:J2ABF0xoWcfHvSOOv7O9_country-us_session-739337@core-residential.evomi.com:1002", "US"},
		{"密码段大写 country-BR", "socks5://user:base_country-BR_session-12@h:1", "BR"},
		{"密码段 country 在末尾", "socks5://user:pw_country-jp@h:1", "JP"},
		{"用户名优先于密码段", "socks5://u-country-kr:pw_country-jp@h:1", "KR"},
		{"两段都没有 country 返回空", "socks5://user:plainpassword@h:1", ""},
		{"密码段 country-united 这种长串不误命中", "socks5://user:pw_country-united-states@h:1", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ParseCountryFromProxyURL(c.raw)
			if got != c.want {
				t.Errorf("ParseCountryFromProxyURL(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}
