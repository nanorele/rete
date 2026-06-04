package sidebar

import (
	"fmt"
	"os"

	"tracto/internal/ui/widgets"
)

var hoverDebug = os.Getenv("TRACTO_HOVER_DEBUG") != ""

var hoverDebugFrame int

func logHoverStates(kind string, labels []string, hovers []*widgets.Hover, first, count int) {
	if !hoverDebug {
		return
	}
	hoverDebugFrame++
	var parts []string
	for i := range hovers {
		if hovers[i].Hovered() {
			parts = append(parts, labels[i])
		}
	}
	if len(parts) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "[hover %s] frame=%d first=%d visCount=%d entered=%v\n", kind, hoverDebugFrame, first, count, parts)
}
