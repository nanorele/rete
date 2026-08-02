//go:build membench

package apptest

import (
	"fmt"
	"runtime"
	"testing"
)

func TestEnvRowCost(t *testing.T) {
	rig := newIdleRig(t)
	seedEnvironments(rig.ui, 1, 2000)
	env := rig.ui.Environments[0]

	a := snap()
	env.InitEditor()
	b := snap()

	rig.ui.EditingEnv = env
	for i := 0; i < 20; i++ {
		rig.frame()
	}
	c := snap()
	churn, _ := rig.churn(60)

	fmt.Printf("\n=== env editor, 2000 vars ===\n")
	fmt.Printf("InitEditor (2000 rows) +%6.2fMB  (%.0f B/var)\n",
		mb(b.HeapAlloc-a.HeapAlloc), float64(b.HeapAlloc-a.HeapAlloc)/2000)
	fmt.Printf("first 20 layouts       +%6.2fMB  (%.0f B/var)\n",
		mb(c.HeapAlloc-b.HeapAlloc), float64(c.HeapAlloc-b.HeapAlloc)/2000)
	fmt.Printf("steady churn            %6.1f KB/frame\n", churn)
	runtime.KeepAlive(rig)
}
