package httpx_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/japansms40-web/gohttpkit/httpx"
)

// TestTransactionJSONTag_与LogField常量同值 锁定 transaction.go 的 json tag 与
// log_fields.go 的 LogField* 常量的单一事实源。
//
// struct tag 不能引用 const，二者只能靠约定同步（见 log_fields.go / transaction.go 的注释）。
// 本测试把「约定」变成硬门禁：改了 tag 没同步常量（或反之）、新增字段忘了登记常量、
// 删了字段留下残余登记，任一情况都会红，避免下游按字段名做的过滤 / 落盘对比静默漂移。
func TestTransactionJSONTag_与LogField常量同值(t *testing.T) {
	// Go 字段名 → 期望的 LogField* 常量。新增 Transaction 字段必须在这里登记，
	// 否则「字段全登记」子测试会失败，提醒同步加常量。
	want := map[string]string{
		"Method":      httpx.LogFieldMethod,
		"URL":         httpx.LogFieldURL,
		"Proxy":       httpx.LogFieldProxy,
		"ExitIP":      httpx.LogFieldExitIP,
		"ASN":         httpx.LogFieldASN,
		"ReqHeaders":  httpx.LogFieldReqHeaders,
		"ReqBody":     httpx.LogFieldReqBody,
		"ReqBodyLen":  httpx.LogFieldReqBodyLen,
		"Status":      httpx.LogFieldStatus,
		"RespHeaders": httpx.LogFieldRespHeaders,
		"RespBody":    httpx.LogFieldRespBody,
		"RespBodyLen": httpx.LogFieldRespBodyLen,
		"DurationMS":  httpx.LogFieldDurationMS,
	}

	typ := reflect.TypeOf(httpx.Transaction{})

	// 正向：每个字段的 json tag（去掉 ,omitempty 等选项）必须等于登记的常量。
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		tag := strings.Split(field.Tag.Get("json"), ",")[0]
		t.Run(field.Name, func(t *testing.T) {
			exp, ok := want[field.Name]
			if !ok {
				t.Fatalf("Transaction 新增字段 %s 未登记：请在 log_fields.go 加 LogField* 常量，并在本测试登记", field.Name)
			}
			t.Logf("%s json=%q 常量=%q", field.Name, tag, exp)
			if tag != exp {
				t.Fatalf("字段 %s 的 json tag %q 与常量 %q 不一致：tag 与 LogField* 必须同值（两处一起改）", field.Name, tag, exp)
			}
		})
	}

	// 反向：登记表里的每个字段都必须真实存在于 Transaction，防止删字段后残留登记。
	for name := range want {
		if _, ok := typ.FieldByName(name); !ok {
			t.Errorf("登记表含 Transaction 已不存在的字段 %s：请同步删除本测试的登记", name)
		}
	}
}
