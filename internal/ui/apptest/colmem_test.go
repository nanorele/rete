//go:build membench

package apptest

import (
	"fmt"
	"runtime"
	"testing"
)

func TestCollectionCost(t *testing.T) {
	rig := newIdleRig(t)

	a := snap()
	seedCollection(rig.ui, 100, 100)
	b := snap()

	for i := 0; i < 20; i++ {
		rig.frame()
	}
	c := snap()
	churn, _ := rig.churn(60)

	n := float64(len(rig.ui.VisibleCols))
	fmt.Printf("\n=== collections, %d visible nodes ===\n", len(rig.ui.VisibleCols))
	fmt.Printf("model + tree      +%6.2fMB  (%.0f B/node)\n", mb(b.HeapAlloc-a.HeapAlloc), float64(b.HeapAlloc-a.HeapAlloc)/n)
	fmt.Printf("first 20 layouts  +%6.2fMB  (%.0f B/node)\n", mb(c.HeapAlloc-b.HeapAlloc), float64(c.HeapAlloc-b.HeapAlloc)/n)
	fmt.Printf("steady churn       %6.1f KB/frame\n", churn)
	runtime.KeepAlive(rig)
}
