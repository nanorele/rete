package environments

import (
	"encoding/json"
	"image/color"
	"io"
	"time"
	"tracto/internal/model"
	"tracto/internal/ui/theme"
	"tracto/internal/ui/widgets"

	"github.com/nanorele/gio/gesture"
	"github.com/nanorele/gio/widget"
)

type EnvVarRow struct {
	KeyEditor widget.Editor
	ValEditor widget.Editor
	Enabled   widget.Bool
	DelBtn    widget.Clickable
}

type EnvironmentUI struct {
	Data       *model.ParsedEnvironment
	SelectBtn  widget.Clickable
	EditBtn    widget.Clickable
	RenameBtn  widget.Clickable
	DupBtn     widget.Clickable
	DelBtn     widget.Clickable
	MenuBtn    widget.Clickable
	MenuOpen   bool
	MenuClickY float32

	List        widget.List
	Rows        []*EnvVarRow
	AddBtn      widget.Clickable
	SaveBtn     widget.Clickable
	BackBtn     widget.Clickable
	NameEditor  widget.Editor
	ColorEditor widget.Editor
	ColorReset  widget.Clickable

	IsRenaming      bool
	RenamingFocused bool
	InlineNameEd    widget.Editor
	LastClickAt     time.Time
	NameScroll      widgets.ScrollLabel
	Drag            gesture.Drag
	Hover           gesture.Hover
	DragOriginY     float32
}

func (ui *EnvironmentUI) InitEditor() {
	ui.NameEditor.SetText(ui.Data.Name)
	ui.ColorEditor.SingleLine = true
	ui.ColorEditor.Submit = true
	ui.ColorEditor.SetText(ui.Data.HighlightColor)
	ui.Rows = nil
	for _, v := range ui.Data.Vars {
		r := &EnvVarRow{}
		r.KeyEditor.SetText(v.Key)
		r.ValEditor.SetText(v.Value)
		r.Enabled.Value = v.Enabled
		ui.Rows = append(ui.Rows, r)
	}
	ui.List.Axis = 1
}

func HighlightColor(env *model.ParsedEnvironment) color.NRGBA {
	if env != nil && env.HighlightColor != "" {
		if c, ok := theme.ParseHex(env.HighlightColor); ok {
			return c
		}
	}
	return theme.Accent
}

func ParseEnvironment(r io.Reader, id string) (*model.ParsedEnvironment, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var ext model.ExtEnvironment
	if err := json.Unmarshal(data, &ext); err != nil {
		return nil, err
	}

	if ext.Name == "" && len(ext.Values) == 0 {
		return nil, io.ErrUnexpectedEOF
	}

	envName := ext.Name
	if envName == "" {
		envName = "Imported Environment"
	}

	var vars []model.EnvVar
	for _, v := range ext.Values {
		enabled := true
		if v.Enabled != nil {
			enabled = *v.Enabled
		}
		vars = append(vars, model.EnvVar{
			Key:     v.Key,
			Value:   v.Value,
			Enabled: enabled,
		})
	}

	return &model.ParsedEnvironment{
		ID:             id,
		Name:           envName,
		Vars:           vars,
		HighlightColor: ext.HighlightColor,
	}, nil
}
