package ws

import "time"

type Dir int

const (
	DirOut Dir = iota
	DirIn
)

func (d Dir) String() string {
	if d == DirOut {
		return "OUT"
	}
	return "IN"
}

type Message struct {
	Time       time.Time
	Dir        Dir
	Opcode     Opcode
	Payload    []byte
	Compressed bool
}

type CloseCode uint16

const (
	CloseNormal           CloseCode = 1000
	CloseGoingAway        CloseCode = 1001
	CloseProtocolError    CloseCode = 1002
	CloseUnsupportedData  CloseCode = 1003
	CloseNoStatusRcvd     CloseCode = 1005
	CloseAbnormalClosure  CloseCode = 1006
	CloseInvalidPayload   CloseCode = 1007
	ClosePolicyViolation  CloseCode = 1008
	CloseMessageTooBig    CloseCode = 1009
	CloseMandatoryExt     CloseCode = 1010
	CloseInternalErr      CloseCode = 1011
	CloseServiceRestart   CloseCode = 1012
	CloseTryAgainLater    CloseCode = 1013
	CloseTLSHandshakeFail CloseCode = 1015
)

func ParseClosePayload(p []byte) (CloseCode, string) {
	if len(p) < 2 {
		return CloseNoStatusRcvd, ""
	}
	code := CloseCode(uint16(p[0])<<8 | uint16(p[1]))
	return code, string(p[2:])
}

func MakeClosePayload(code CloseCode, reason string) []byte {
	buf := make([]byte, 2+len(reason))
	buf[0] = byte(code >> 8)
	buf[1] = byte(code)
	copy(buf[2:], reason)
	return buf
}
