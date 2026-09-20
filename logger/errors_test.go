package logger

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

func assertFileSetup(t *testing.T, err error, op, path string) {
	t.Helper()
	var got *FileSetupError
	if !errors.As(err, &got) {
		t.Fatalf("err = %v (%T), want *FileSetupError", err, err)
	}
	if got.Op != op {
		t.Fatalf("Op = %q, want %q", got.Op, op)
	}
	if path != "" && got.Path != path {
		t.Fatalf("Path = %q, want %q", got.Path, path)
	}
	t.Logf("errors.As → *FileSetupError Op=%q Path=%q err=%v", got.Op, got.Path, err)
}

func TestFileSetupError_字段与文案(t *testing.T) {
	err := &FileSetupError{Path: "/tmp/a.log", Op: "open", Err: io.EOF}
	assertFileSetup(t, err, "open", "/tmp/a.log")
	want := `logger: open "/tmp/a.log": EOF`
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestFileSetupError_底层为空(t *testing.T) {
	err := &FileSetupError{Path: "/tmp/a.log", Op: "mkdir"}
	assertFileSetup(t, err, "mkdir", "/tmp/a.log")
	want := `logger: mkdir "/tmp/a.log"`
	t.Logf("Error() = %q", err.Error())
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}
}

func TestFileSetupError_nil接收者(t *testing.T) {
	got := (*FileSetupError)(nil).Error()
	t.Logf("(*FileSetupError)(nil).Error() = %q", got)
	if got != "logger: file setup failed <nil>" {
		t.Errorf("Error() = %q", got)
	}
	if (*FileSetupError)(nil).Unwrap() != nil {
		t.Fatal("nil Unwrap 应返回 nil")
	}
}

func TestFileSetupError_包装后仍可As并Unwrap(t *testing.T) {
	inner := io.ErrClosedPipe
	wrapped := fmt.Errorf("boot: %w", &FileSetupError{Path: "x.log", Op: "open", Err: inner})
	t.Logf("wrapped = %v", wrapped)
	assertFileSetup(t, wrapped, "open", "x.log")
	if !errors.Is(wrapped, inner) {
		t.Fatal("应能 errors.Is 到底层 io 错误")
	}
}
