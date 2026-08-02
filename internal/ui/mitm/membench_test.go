//go:build membench

package mitm

import (
	"fmt"
	"image"
	"runtime"
	"testing"
	"time"
)

func benchMB(v uint64) float64 { return float64(v) / (1 << 20) }

func kbPerFrame(rig *uiRig, frames int) float64 {
	for i := 0; i < 5; i++ {
		rig.frame()
	}
	var a, b runtime.MemStats
	runtime.ReadMemStats(&a)
	for i := 0; i < frames; i++ {
		rig.frame()
	}
	runtime.ReadMemStats(&b)
	return float64(b.TotalAlloc-a.TotalAlloc) / float64(frames) / (1 << 10)
}

func benchBody(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a' + byte(i%26) + seed%3
	}
	return b
}

func seedBenchFlows(rig *uiRig, n, bodySize int) {
	for i := 0; i < n; i++ {
		rig.s.Store.Add(&Flow{
			Kind:       FlowHTTP,
			ClientAddr: "127.0.0.1:5000",
			Started:    time.Unix(1700000000, 0),
			Ended:      time.Unix(1700000001, 0),
			Scheme:     "https",
			Method:     "POST",
			Host:       fmt.Sprintf("api%d.example.com", i%17),
			Path:       fmt.Sprintf("/v2/resource/%d", i),
			URL:        fmt.Sprintf("https://api%d.example.com/v2/resource/%d", i%17, i),
			Status:     "200 OK",
			StatusCode: 200,
			ReqHeaders: [][2]string{{"Content-Type", "application/json"}, {"Accept", "*/*"}},
			RespHeaders: [][2]string{
				{"Content-Type", "application/json"}, {"Server", "nginx"}, {"Date", "now"},
			},
			ReqBody:  benchBody(bodySize/8, byte(i)),
			RespBody: benchBody(bodySize, byte(i)),
			ReqSize:  int64(bodySize / 8),
			RespSize: int64(bodySize),
		})
	}
}

func TestMITMHistoryChurn(t *testing.T) {
	fmt.Printf("\n=== MITM history/inspector churn ===\n")
	for _, c := range []struct {
		flows    int
		bodySize int
		selected bool
	}{
		{0, 0, false},
		{200, 4 << 10, false},
		{200, 4 << 10, true},
		{2000, 4 << 10, true},
		{200, 1 << 20, true},
	} {
		rig := newUIRig(t, image.Pt(1280, 720))
		rig.s.View = ViewHistory
		seedBenchFlows(rig, c.flows, c.bodySize)
		if c.selected {
			rig.s.Selected = uint64(c.flows / 2)
		}
		runtime.GC()
		var live runtime.MemStats
		runtime.ReadMemStats(&live)
		churn := kbPerFrame(rig, 40)
		fmt.Printf("flows=%4d body=%7s selected=%-5v  churn=%9.1f KB/frame  heap=%7.1fMB\n",
			c.flows, benchSizeName(c.bodySize), c.selected, churn, benchMB(live.HeapAlloc))
		runtime.KeepAlive(rig)
	}
}

func TestMITMWSChurn(t *testing.T) {
	fmt.Printf("\n=== MITM websocket view churn ===\n")
	for _, c := range []struct {
		msgs     int
		size     int
		selected bool
	}{
		{0, 0, false},
		{500, 1 << 10, false},
		{500, 1 << 10, true},
		{5000, 1 << 10, true},
		{500, 256 << 10, true},
	} {
		rig := newUIRig(t, image.Pt(1280, 720))
		rig.s.View = ViewWebSockets
		for i := 0; i < c.msgs; i++ {
			rig.s.Proxy.WS.Add(&WSMessage{
				FlowID:   uint64(i%20 + 1),
				URL:      "wss://example.com/socket",
				ToServer: i%2 == 0,
				Opcode:   0x1,
				Payload:  benchBody(c.size, byte(i)),
				Time:     time.Unix(1700000000, 0),
			})
		}
		if c.selected {
			rig.s.WSSelected = uint64(c.msgs / 2)
		}
		runtime.GC()
		churn := kbPerFrame(rig, 40)
		fmt.Printf("msgs=%4d size=%7s selected=%-5v  churn=%9.1f KB/frame\n",
			c.msgs, benchSizeName(c.size), c.selected, churn)
		runtime.KeepAlive(rig)
	}
}

func benchSizeName(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%dMB", n>>20)
	case n >= 1<<10:
		return fmt.Sprintf("%dKB", n>>10)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
