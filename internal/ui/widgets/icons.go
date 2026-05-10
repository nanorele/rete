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

func init() {
	IconClose, _ = widget.NewIcon(icons.NavigationClose)
	IconSettings, _ = widget.NewIcon(icons.ActionSettings)
	IconSave, _ = widget.NewIcon(icons.ContentSave)
	IconBack, _ = widget.NewIcon(icons.NavigationArrowBack)
	IconAddReq, _ = widget.NewIcon(icons.ActionNoteAdd)
	IconAddFld, _ = widget.NewIcon(icons.FileCreateNewFolder)
	IconRename, _ = widget.NewIcon(icons.EditorModeEdit)
	IconDup, _ = widget.NewIcon(icons.ContentContentCopy)
	IconDel, _ = widget.NewIcon(icons.ActionDelete)
	IconSearch, _ = widget.NewIcon(icons.ActionSearch)
	IconBug, _ = widget.NewIcon(icons.ActionBugReport)
	IconDropDown, _ = widget.NewIcon(icons.NavigationArrowDropDown)
	IconChevronR, _ = widget.NewIcon(icons.NavigationChevronRight)
	IconChevronL, _ = widget.NewIcon(icons.NavigationChevronLeft)
	IconChevronD, _ = widget.NewIcon(icons.HardwareKeyboardArrowDown)
	IconRefresh, _ = widget.NewIcon(icons.NavigationRefresh)
}
