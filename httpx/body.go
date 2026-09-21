package httpx

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// body.go —— 请求体编码、响应 JSON 解码、日志 body 截断。
// 独立成文件：编解码与截断是纯函数，不依赖 Client / 链，单测可以不搭服务器。

// EncodeRequestBody 把请求体编码为字节串。
// 给 Client.Do / PostJSON 以及自组请求的调用方：进链前把多种 body 形态收成 []byte。
// 输入 body：
//
//	nil → 不带体；[]byte → 原样返回同一份切片（不复制）；
//	string → 转成字节（手工拼的表单/JSON，保持参数顺序）；
//	url.Values → 标准表单编码（key 按字典序）；
//	其它 json.Marshaler / struct / map → JSON 编码。
//
// 返回：成功为编码后的字节；nil body 为 (nil, nil)；JSON 失败为 *RequestBodyEncodeError。
// 例：EncodeRequestBody(nil) → (nil, nil)；
// EncodeRequestBody(func(){}) → *RequestBodyEncodeError{BodyType:"func()"}。
// 为什么保留 string 分支：高保真复刻场景下表单参数的【顺序】是指纹的一部分，
// url.Values.Encode() 会按字典序重排，只能靠调用方手工拼串再原样发出。
func EncodeRequestBody(body any) ([]byte, error) {
	switch v := body.(type) {
	case nil:
		return nil, nil
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	case url.Values:
		return []byte(v.Encode()), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, &RequestBodyEncodeError{BodyType: fmt.Sprintf("%T", body), Err: err}
		}
		return data, nil
	}
}

// DecodeResponse 把响应体解析为指定 JSON 类型。
// 给拿到 Do 返回字节后要反序列化的调用方，不依赖 Client。
// 输入 data：响应体字节，可为 nil/空（走 json.Unmarshal 的空输入语义）；
// 输入 v：解码目标指针，不会先清空再写；传入值类型会得到 json 的类型错误。
// 返回：成功 nil；失败 *ResponseJSONDecodeError{TargetType, Err}。
// 例：DecodeResponse([]byte(`{"ok":true}`), &m) → nil；
// DecodeResponse([]byte(`{`), &m) → *ResponseJSONDecodeError。
func DecodeResponse(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return &ResponseJSONDecodeError{TargetType: fmt.Sprintf("%T", v), Err: err}
	}
	return nil
}

// LogBodyMaxBytes 协议 body 在日志中的默认最大输出长度。
//
// 现实教训：某些接口的响应体可达 ~800KB，全量打印会让单条 JSON 日志接近 1MB，
// 几百个 worker 并发时直接撑爆磁盘与日志聚合系统（甚至触发容器 OOM）。
// 超过此长度只保留前 N 字节并附 ...(truncated, total=N) 占位。
const LogBodyMaxBytes = 4096

// TruncateBodyForLog 限制协议 body 在日志中的字节数。
// 给拦截器与诊断日志：打协议体时必须走这里，避免几百 KB 的响应撑爆日志。
// 输入 b：原始字节，可为 nil；limit <= 0 表示不截断。
// 返回：未超限或不截断时原样返回同一份切片（不复制）；超限则新切片，前 limit 字节 + 占位后缀。
// 例：TruncateBodyForLog([]byte("hi"), 10) → []byte("hi")；
// TruncateBodyForLog(make([]byte, 5000), 4) → 前 4 字节 + "...(truncated, total=5000)"。
// 调用方应同时输出原始长度（slog.Int("..._len", len(b))），便于事后核对。
func TruncateBodyForLog(b []byte, limit int) []byte {
	if limit <= 0 || len(b) <= limit {
		return b
	}
	suffix := []byte(fmt.Sprintf("...(truncated, total=%d)", len(b)))
	out := make([]byte, 0, limit+len(suffix))
	out = append(out, b[:limit]...)
	out = append(out, suffix...)
	return out
}
