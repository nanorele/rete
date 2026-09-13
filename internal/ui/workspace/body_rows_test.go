package workspace

import (
	"rete/internal/model"
	"testing"

	"github.com/nanorele/gio/widget/material"
)

func TestFormPartRowFileMatchesTextHeight(t *testing.T) {
	th := material.NewTheme()
	textPart := NewFormPart("k", "v", model.FormPartText, "", 0)
	filePart := NewFormPart("k", "", model.FormPartFile, "C:\\tmp\\report_with_a_long_name.bin", 12345)
	emptyFile := NewFormPart("k", "", model.FormPartFile, "", 0)

	textH := formPartRow(makeBodyTestGtx(), th, textPart, nil).Size.Y
	fileH := formPartRow(makeBodyTestGtx(), th, filePart, nil).Size.Y
	emptyH := formPartRow(makeBodyTestGtx(), th, emptyFile, nil).Size.Y
	if fileH != textH {
		t.Errorf("file row height %d must equal text row height %d", fileH, textH)
	}
	if emptyH != textH {
		t.Errorf("empty file row height %d must equal text row height %d", emptyH, textH)
	}
}

func TestURLEncodedRowMatchesFormRowHeight(t *testing.T) {
	th := material.NewTheme()
	formH := formPartRow(makeBodyTestGtx(), th, NewFormPart("k", "v", model.FormPartText, "", 0), nil).Size.Y
	ueH := urlEncodedRow(makeBodyTestGtx(), th, NewURLEncodedPart("k", "v"), nil).Size.Y
	if ueH != formH {
		t.Errorf("urlencoded row height %d must equal form-data row height %d", ueH, formH)
	}
}

func TestFormPartKindToggleKeepsFile(t *testing.T) {
	rig := newVStackRig()
	rig.tab.BodyType = model.BodyFormData
	part := NewFormPart("avatar", "", model.FormPartFile, "C:\\tmp\\a.png", 99)
	rig.tab.FormParts = []*FormDataPart{part}
	for i := 0; i < 3; i++ {
		rig.frame()
	}

	part.KindBtn.Click()
	rig.frame()
	rig.frame()
	if part.Kind != model.FormPartText {
		t.Fatalf("first toggle must switch to text, got %v", part.Kind)
	}
	part.KindBtn.Click()
	rig.frame()
	rig.frame()
	if part.Kind != model.FormPartFile {
		t.Fatalf("second toggle must switch back to file, got %v", part.Kind)
	}
	if part.FilePath != "C:\\tmp\\a.png" || part.FileSize != 99 {
		t.Errorf("chosen file must survive text/file round-trip: path %q size %d", part.FilePath, part.FileSize)
	}
}
