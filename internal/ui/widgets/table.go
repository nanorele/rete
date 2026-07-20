package widgets

import (
	"tracto/internal/ui/theme"

	"github.com/nanorele/gio-x/component"
	"github.com/nanorele/gio/layout"
	"github.com/nanorele/gio/widget/material"
)

const TableHInset = component.TableHInset

type TableColumn = component.TableColumn

type Table struct{ *component.ColumnTable }

func NewTable(cols []TableColumn) *Table {
	return &Table{component.NewColumnTable(cols)}
}

func (t *Table) Header(gtx layout.Context, th *material.Theme) layout.Dimensions {
	return t.ColumnTable.Header(gtx, th, tableColors())
}

func tableColors() component.TableColors {
	return component.TableColors{
		HeaderBg:     theme.BgDark,
		HeaderFg:     theme.FgMuted,
		Separator:    theme.BorderLight,
		ResizeHandle: theme.Accent,
	}
}
