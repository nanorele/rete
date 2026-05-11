package settings

import (
	"tracto/internal/model"

	"github.com/nanorele/gio/app"
	"github.com/nanorele/gio/widget/material"
)

type Host struct {
	Theme   *material.Theme
	Window  *app.Window
	Current *model.AppSettings
	Open    *bool
	OnClose func()
	OnSave  func()
}
