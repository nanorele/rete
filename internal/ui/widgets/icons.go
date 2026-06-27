package widgets

import (
	"github.com/nanorele/gio/widget"
	"golang.org/x/exp/shiny/iconvg"
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
	IconPlay       *widget.Icon
	IconStop       *widget.Icon
	IconBatch      *widget.Icon
	IconFlow       *widget.Icon
	IconSplit      *widget.Icon
	IconDelay      *widget.Icon
	IconHistory    *widget.Icon
	IconTune       *widget.Icon
	IconFit        *widget.Icon
	IconFolderOpen *widget.Icon
	IconPause      *widget.Icon
	IconNext       *widget.Icon
	IconNetlimit   *widget.Icon
	IconDownload   *widget.Icon
	IconUpload     *widget.Icon
	IconLab        *widget.Icon
	IconCheck      *widget.Icon
)

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic("widgets: failed to load icon: " + err.Error())
	}
	return ic
}

func labIcon() *widget.Icon {
	var e iconvg.Encoder
	e.Reset(iconvg.Metadata{ViewBox: iconvg.DefaultViewBox, Palette: iconvg.DefaultPalette})
	e.StartPath(0, -7, -21)
	e.AbsLineTo(7, -21)
	e.AbsLineTo(7, -17)
	e.AbsLineTo(5, -17)
	e.AbsLineTo(5, -5)
	e.AbsLineTo(22, 19)
	e.AbsLineTo(22, 22)
	e.AbsLineTo(-22, 22)
	e.AbsLineTo(-22, 19)
	e.AbsLineTo(-5, -5)
	e.AbsLineTo(-5, -17)
	e.AbsLineTo(-7, -17)
	e.ClosePathEndPath()
	if data, err := e.Bytes(); err == nil {
		if ic, err := widget.NewIcon(data); err == nil {
			return ic
		}
	}
	return mustIcon(icons.ImageColorize)
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
	IconBatch = mustIcon(icons.AVPlaylistPlay)
	IconFlow = mustIcon(icons.EditorLinearScale)
	IconSplit = mustIcon(icons.CommunicationCallSplit)
	IconDelay = mustIcon(icons.ActionSchedule)
	IconHistory = mustIcon(icons.ActionHistory)
	IconTune = mustIcon(icons.ImageTune)
	IconFit = mustIcon(icons.ImageCropFree)
	IconFolderOpen = mustIcon(icons.FileFolderOpen)
	IconPause = mustIcon(icons.AVPause)
	IconNext = mustIcon(icons.AVSkipNext)
	IconNetlimit = mustIcon(icons.NotificationNetworkCheck)
	IconDownload = mustIcon(icons.FileFileDownload)
	IconUpload = mustIcon(icons.FileFileUpload)
	IconLab = labIcon()
	IconCheck = mustIcon(icons.NavigationCheck)
}
