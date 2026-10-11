package logger

import "fmt"

// FileSetupError 日志文件建不出来或打不开。
// Init 在落盘失败时返回本类型（Op=open），进程日志状态保持不变。
// 判定请用 errors.As，不要扫文案。
type FileSetupError struct {
	// Path 实际尝试使用的文件路径（空输入已被回落成默认路径）。
	Path string
	// Op 失败步骤，目前只有 open：建目录或打开文件。
	Op string
	// Err 底层错误。
	Err error
}

// Error 实现 error。
// 输入：接收者可为 nil。
// 返回：nil → "logger: file setup failed <nil>"；
// 否则 → `logger: open "/path": ...`。
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
