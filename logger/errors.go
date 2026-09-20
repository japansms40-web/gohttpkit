package logger

import "fmt"

// FileSetupError 创建或打开日志文件失败。
// mustFileWriter 在进程启动期构造默认 logger，失败比带着半残 handler 继续更安全，
// 因此以 panic 抛出本类型。判定请用 errors.As，不要扫文案。
type FileSetupError struct {
	// Path 实际尝试使用的文件路径（空输入已被回落成默认路径）。
	Path string
	// Op 失败步骤：mkdir 或 open。
	Op string
	// Err 底层错误。
	Err error
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "logger: file setup failed <nil>"；
// 否则 → `logger: mkdir "/path": ...` 或 `logger: open "/path": ...`。
func (e *FileSetupError) Error() string {
	if e == nil {
		return "logger: file setup failed <nil>"
	}
	if e.Err == nil {
		return fmt.Sprintf("logger: %s %q", e.Op, e.Path)
	}
	return fmt.Sprintf("logger: %s %q: %v", e.Op, e.Path, e.Err)
}

// Unwrap 交出底层错误。
// 输入：接收者可为 nil。
// 返回：nil 接收者或 Err==nil → nil；否则是构造时传入的 err。
func (e *FileSetupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
