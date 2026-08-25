package httpx

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// EncodeRequestBody 把请求体编码为字节串，支持：
//
//	nil                                  → (nil, nil)，不带体
//	[]byte                               → 原样（自己序列化好的场景）
//	string                               → 原样字节（手工拼的表单/JSON，保持参数顺序）
//	url.Values                           → 标准表单编码（key 按字典序）
//	map[string]any / map[string]string   → JSON 编码
//	其它实现了 json.Marshaler 或任意 struct → JSON 编码
//
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
			return nil, fmt.Errorf("httpx: 请求体 JSON 编码失败(%T): %w", body, err)
		}
		return data, nil
	}
}

// DecodeResponse 把响应体解析为指定类型。
func DecodeResponse(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("httpx: 响应体 JSON 解析失败: %w", err)
	}
	return nil
}

// LogBodyMaxBytes 协议 body 在日志中的默认最大输出长度。
//
// 现实教训：某些接口的响应体可达 ~800KB，全量打印会让单条 JSON 日志接近 1MB，
// 几百个 worker 并发时直接撑爆磁盘与日志聚合系统（甚至触发容器 OOM）。
// 超过此长度只保留前 N 字节并附 ...(truncated, total=N) 占位。
const LogBodyMaxBytes = 4096

// TruncateBodyForLog 限制协议 body 在日志中的字节数；limit <= 0 表示不截断。
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
