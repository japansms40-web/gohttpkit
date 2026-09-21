package logger_test

import (
	"testing"

	"github.com/japansms40-web/gohttpkit/logger"
)

type accountEvent string

func (e accountEvent) Name() string { return string(e) }

func TestEvent_外部包可实现(t *testing.T) {
	var e logger.Event = accountEvent("account.login.failed")
	attr := logger.Attr(e)
	t.Logf("name=%q attr=%v", e.Name(), attr)
	if e.Name() != "account.login.failed" || attr.Key != "event" ||
		attr.Value.String() != e.Name() {
		t.Fatalf("name=%q attr=%v", e.Name(), attr)
	}
}
