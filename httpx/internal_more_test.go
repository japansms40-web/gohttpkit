package httpx

import (
	"testing"
	"time"
)

func TestSideChannel与Terminal方法体(t *testing.T) {
	// 空方法从外部包调用常被内联掉，cover 记 0%；同包直接调才能焊到行。
	SideChannelMarker{}.SideChannel()
	TerminalMarker{}.Terminal()
	t.Logf("marker 方法体已执行")
}

func TestNewTransport_headerTimeout非正回落15s(t *testing.T) {
	for _, ht := range []time.Duration{0, -time.Second} {
		tr, err := newTransport("", ht)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("headerTimeout=%v → ResponseHeaderTimeout=%v", ht, tr.ResponseHeaderTimeout)
		if tr.ResponseHeaderTimeout != defaultResponseHeaderTimeout {
			t.Fatalf("应回落 %v，得到 %v", defaultResponseHeaderTimeout, tr.ResponseHeaderTimeout)
		}
	}
}
