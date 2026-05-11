package widgets

import (
	"github.com/nanorele/gio/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var (
	IconClose    *widget.Icon
	IconSettings *widget.Icon
	IconSave     *widget.Icon
	IconBack     *widget.Icon
	IconAddReq   *widget.Icon
	IconAddFld   *widget.Icon
	IconRename   *widget.Icon
	IconDup      *widget.Icon
	IconDel      *widget.Icon
	IconSearch   *widget.Icon
	IconBug      *widget.Icon
	IconDropDown *widget.Icon
	IconChevronR *widget.Icon
	IconChevronL *widget.Icon
	IconChevronD *widget.Icon
	IconRefresh  *widget.Icon
)

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic("widgets: failed to load icon: " + err.Error())
	}
	return ic
}

func init() {
	IconClose = mustIcon(icons.NavigationClose)
	IconSettings = mustIcon(icons.ActionSettings)
	IconSave = mustIcon(icons.ContentSave)
	IconBack = mustIcon(icons.NavigationArrowBack)
	IconAddReq = mustIcon(icons.ActionNoteAdd)
	IconAddFld = mustIcon(icons.FileCreateNewFolder)
	IconRename = mustIcon(icons.EditorModeEdit)
	IconDup = mustIcon(icons.ContentContentCopy)
	IconDel = mustIcon(icons.ActionDelete)
	IconSearch = mustIcon(icons.ActionSearch)
	IconBug = mustIcon(icons.ActionBugReport)
	IconDropDown = mustIcon(icons.NavigationArrowDropDown)
	IconChevronR = mustIcon(icons.NavigationChevronRight)
	IconChevronL = mustIcon(icons.NavigationChevronLeft)
	IconChevronD = mustIcon(icons.HardwareKeyboardArrowDown)
	IconRefresh = mustIcon(icons.NavigationRefresh)
}
