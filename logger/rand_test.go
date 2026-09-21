package logger

import (
	"errors"
	"regexp"
	"testing"
)

func TestNewRandomHex_rand失败走UnixNano兜底(t *testing.T) {
	orig := readRandom
	readRandom = func([]byte) (int, error) { return 0, errors.New("no entropy") }
	t.Cleanup(func() { readRandom = orig })

	hexRe := regexp.MustCompile(`^[0-9a-f]+$`)
	for _, c := range []struct{ nBytes, hexWidth int }{{8, 16}, {4, 8}} {
		got := newRandomHex(c.nBytes, c.hexWidth)
		t.Logf("nBytes=%d hexWidth=%d → %q (len=%d)", c.nBytes, c.hexWidth, got, len(got))
		if !hexRe.MatchString(got) {
			t.Errorf("兜底不是 hex: %q", got)
		}
		if len(got) < c.hexWidth {
			t.Errorf("兜底短于 hexWidth=%d: %q", c.hexWidth, got)
		}
	}

	tid := NewTraceID()
	sid := NewSpanID()
	t.Logf("NewTraceID=%q NewSpanID=%q", tid, sid)
	if !hexRe.MatchString(tid) || !hexRe.MatchString(sid) {
		t.Errorf("Trace/Span 兜底不是 hex: %q %q", tid, sid)
	}
}
