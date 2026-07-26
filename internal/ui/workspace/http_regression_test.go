package workspace

import (
	"net/http"
	"testing"
)

func TestDecompressBodyNilInputs(t *testing.T) {
	if got := decompressBody(nil); got != nil {
		t.Errorf("decompressBody(nil) = %v, want nil", got)
	}
	if got := decompressBody(&http.Response{}); got != nil {
		t.Errorf("decompressBody with nil Body = %v, want nil", got)
	}
}
