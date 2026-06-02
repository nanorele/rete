package widgets

import (
	"github.com/nanorele/gio/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var (
	IconClose      *widget.Icon
	IconSettings   *widget.Icon
	IconSave       *widget.Icon
	IconBack       *widget.Icon
	IconAdd        *widget.Icon
	IconAddReq     *widget.Icon
	IconAddFld     *widget.Icon
	IconRename     *widget.Icon
	IconDup        *widget.Icon
	IconDel        *widget.Icon
	IconClear      *widget.Icon
	IconSearch     *widget.Icon
	IconBug        *widget.Icon
	IconDropDown   *widget.Icon
	IconChevronR   *widget.Icon
	IconChevronL   *widget.Icon
	IconChevronD   *widget.Icon
	IconExpandLess *widget.Icon
	IconExpandMore *widget.Icon
	IconMore       *widget.Icon
	IconRefresh    *widget.Icon
	IconRequests   *widget.Icon
	IconMITM       *widget.Icon
	IconShield     *widget.Icon
	IconUAC        *widget.Icon
	IconPlay       *widget.Icon
	IconStop       *widget.Icon
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
	IconAdd = mustIcon(icons.ContentAdd)
	IconAddReq = mustIcon(icons.ActionNoteAdd)
	IconAddFld = mustIcon(icons.FileCreateNewFolder)
	IconRename = mustIcon(icons.EditorModeEdit)
	IconDup = mustIcon(icons.ContentContentCopy)
	IconDel = mustIcon(icons.ActionDelete)
	IconClear = mustIcon(icons.ContentDeleteSweep)
	IconSearch = mustIcon(icons.ActionSearch)
	IconBug = mustIcon(icons.ActionBugReport)
	IconDropDown = mustIcon(icons.NavigationArrowDropDown)
	IconChevronR = mustIcon(icons.NavigationChevronRight)
	IconChevronL = mustIcon(icons.NavigationChevronLeft)
	IconChevronD = mustIcon(icons.HardwareKeyboardArrowDown)
	IconExpandLess = mustIcon(icons.NavigationExpandLess)
	IconExpandMore = mustIcon(icons.NavigationExpandMore)
	IconMore = mustIcon(icons.NavigationMoreVert)
	IconRefresh = mustIcon(icons.NavigationRefresh)
	IconRequests = mustIcon(icons.CommunicationCallMade)
	IconMITM = mustIcon(icons.ActionSwapHoriz)
	IconShield = mustIcon(icons.ActionVerifiedUser)
	IconPlay = mustIcon(icons.AVPlayArrow)
	IconStop = mustIcon(icons.AVStop)
}
