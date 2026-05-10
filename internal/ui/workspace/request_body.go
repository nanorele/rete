package workspace

import (
	"tracto/internal/model"

	"github.com/nanorele/gio/widget"
)

type FormDataPart struct {
	Key       widget.Editor
	Kind      model.FormPartKind
	Value     widget.Editor
	FilePath  string
	FileSize  int64
	KindBtn   widget.Clickable
	ChooseBtn widget.Clickable
	DelBtn    widget.Clickable
}

type URLEncodedPart struct {
	Key    widget.Editor
	Value  widget.Editor
	DelBtn widget.Clickable
}

func NewFormPart(key, value string, kind model.FormPartKind, filePath string, fileSize int64) *FormDataPart {
	p := &FormDataPart{Kind: kind, FilePath: filePath, FileSize: fileSize}
	p.Key.SingleLine = true
	p.Value.SingleLine = true
	p.Key.SetText(key)
	p.Value.SetText(value)
	return p
}

func NewURLEncodedPart(key, value string) *URLEncodedPart {
	p := &URLEncodedPart{}
	p.Key.SingleLine = true
	p.Value.SingleLine = true
	p.Key.SetText(key)
	p.Value.SetText(value)
	return p
}
