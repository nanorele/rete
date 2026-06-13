//go:build windows

package netlimit

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modiphlpapi      = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetIfTable2  = modiphlpapi.NewProc("GetIfTable2")
	procFreeMibTable = modiphlpapi.NewProc("FreeMibTable")
)

const (
	ifTypeSoftwareLoopback = 24
	ifOperStatusUp         = 1
)

type mibIfRow2 struct {
	InterfaceLuid               uint64
	InterfaceIndex              uint32
	InterfaceGuid               windows.GUID
	Alias                       [257]uint16
	Description                 [257]uint16
	PhysicalAddressLength       uint32
	PhysicalAddress             [32]byte
	PermanentPhysicalAddress    [32]byte
	Mtu                         uint32
	Type                        uint32
	TunnelType                  uint32
	MediaType                   uint32
	PhysicalMediumType          uint32
	AccessType                  uint32
	DirectionType               uint32
	InterfaceAndOperStatusFlags uint8
	OperStatus                  uint32
	AdminStatus                 uint32
	MediaConnectState           uint32
	NetworkGuid                 windows.GUID
	ConnectionType              uint32
	TransmitLinkSpeed           uint64
	ReceiveLinkSpeed            uint64
	InOctets                    uint64
	InUcastPkts                 uint64
	InNUcastPkts                uint64
	InDiscards                  uint64
	InErrors                    uint64
	InUnknownProtos             uint64
	InUcastOctets               uint64
	InMulticastOctets           uint64
	InBroadcastOctets           uint64
	OutOctets                   uint64
	OutUcastPkts                uint64
	OutNUcastPkts               uint64
	OutDiscards                 uint64
	OutErrors                   uint64
	OutUcastOctets              uint64
	OutMulticastOctets          uint64
	OutBroadcastOctets          uint64
	OutQLen                     uint64
}

type mibIfTable2 struct {
	NumEntries uint32
	_          uint32
	Table      [1]mibIfRow2
}

type winMonitor struct {
	sniff *winDivertSniffer
}

func newMonitor() Monitor {
	return &winMonitor{}
}

func (m *winMonitor) SystemCounters() (rx, tx uint64, err error) {
	var tbl *mibIfTable2
	r, _, _ := procGetIfTable2.Call(uintptr(unsafe.Pointer(&tbl)))
	if r != 0 {
		return 0, 0, fmt.Errorf("GetIfTable2 failed: %d", r)
	}
	defer procFreeMibTable.Call(uintptr(unsafe.Pointer(tbl)))

	n := tbl.NumEntries
	base := unsafe.Pointer(&tbl.Table[0])
	sz := unsafe.Sizeof(tbl.Table[0])
	for i := uint32(0); i < n; i++ {
		row := (*mibIfRow2)(unsafe.Add(base, uintptr(i)*sz))
		if row.Type == ifTypeSoftwareLoopback || row.OperStatus != ifOperStatusUp {
			continue
		}
		rx += row.InOctets
		tx += row.OutOctets
	}
	return rx, tx, nil
}

func (m *winMonitor) AppCounters(pid int32) (rx, tx uint64, err error) {
	if m.sniff == nil {
		s, err := newWinDivertSniffer()
		if err != nil {
			return 0, 0, err
		}
		m.sniff = s
	}
	return m.sniff.counters(pid)
}

func (m *winMonitor) Close() error {
	if m.sniff != nil {
		return m.sniff.close()
	}
	return nil
}
