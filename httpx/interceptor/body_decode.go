package interceptor

import (
	"compress/flate"
	"compress/gzip"
	"io"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"

	"github.com/japansms40-web/gohttpkit/errors"
	"github.com/japansms40-web/gohttpkit/httpx"
	"github.com/japansms40-web/gohttpkit/logger"
)

type bodyDecodeInterceptor struct{}

// NewBodyDecodeInterceptor 读取响应体并按 content-encoding 解码（zstd / gzip / deflate / br）。
// 给默认链：解码能力必须覆盖 accept-encoding 声明的集合，否则下游拿到乱码。
// 输入：无。返回可放入链的 Interceptor。
// 位置：在 retry 之外（解码错误不触发重试），在 statusCodeCache 之外。
func NewBodyDecodeInterceptor() httpx.Interceptor { return &bodyDecodeInterceptor{} }

// Intercept 先 Proceed，再读体并解压。
// 输入 ch：不改 Request；总是 Close Raw.Body（gzip reader 也会关）。
// 返回：内层错误原样穿透；Raw==nil 视为已填好体，原样返回；
// reader 初始化失败 → *httpx.ContentEncodingError；
// 可重试读错 → *errors.RetryableError；普通读错 → *httpx.ReadResponseBodyError。
func (i *bodyDecodeInterceptor) Intercept(ch *httpx.Chain) (*httpx.Response, error) {
	resp, err := ch.Proceed()
	if err != nil {
		return nil, err
	}
	if resp.Raw == nil {
		return resp, nil
	}

	raw := resp.Raw
	defer func() {
		if cerr := raw.Body.Close(); cerr != nil {
			logger.Error(ch.Request().Ctx, "failed to close response body", logger.Err(cerr))
		}
	}()

	encoding, _ := httpx.ParseContentEncoding(resp.Header.Get("content-encoding"))
	var reader io.Reader = raw.Body
	switch encoding {
	case httpx.EncodingZstd:
		zr, zerr := zstd.NewReader(raw.Body)
		if zerr != nil {
			return nil, &httpx.ContentEncodingError{Encoding: httpx.EncodingZstd, Err: zerr}
		}
		defer zr.Close()
		reader = zr
	case httpx.EncodingGzip:
		gz, gerr := gzip.NewReader(raw.Body)
		if gerr != nil {
			return nil, &httpx.ContentEncodingError{Encoding: httpx.EncodingGzip, Err: gerr}
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	case httpx.EncodingDeflate:
		fr := flate.NewReader(raw.Body)
		defer func() { _ = fr.Close() }()
		reader = fr
	case httpx.EncodingBr:
		reader = brotli.NewReader(raw.Body)
	}

	body, rerr := io.ReadAll(reader)
	if rerr != nil {
		if errors.IsRetryableNetworkError(rerr) {
			return nil, &errors.RetryableError{Err: rerr, Attempts: 1, LastError: rerr}
		}
		return nil, &httpx.ReadResponseBodyError{Encoding: encoding, Err: rerr}
	}

	resp.Body = body
	resp.Raw = nil
	return resp, nil
}
