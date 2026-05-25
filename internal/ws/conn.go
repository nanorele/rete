package ws

import (
	"bufio"
	"crypto/rand"
	"errors"
	"net"
	"sync"
)

var ErrConnClosed = errors.New("ws: connection closed")

type Conn struct {
	rwc      net.Conn
	br       *bufio.Reader
	isClient bool
	reasm    Reassembler
	inflater *Inflater
	deflater *Deflater

	writeMu sync.Mutex
	closed  bool
	closeMu sync.Mutex
}

func NewConn(rwc net.Conn, br *bufio.Reader, isClient bool, ext ExtParams) (*Conn, error) {
	if br == nil {
		br = bufio.NewReader(rwc)
	}
	c := &Conn{
		rwc:      rwc,
		br:       br,
		isClient: isClient,
	}
	if ext.Negotiated {
		c.inflater = NewInflater(serverNoContextForReader(isClient, ext))
		df, err := NewDeflater(serverNoContextForWriter(isClient, ext))
		if err != nil {
			return nil, err
		}
		c.deflater = df
	}
	return c, nil
}

func serverNoContextForReader(isClient bool, ext ExtParams) bool {
	if isClient {
		return ext.ServerNoContextTakeover
	}
	return ext.ClientNoContextTakeover
}

func serverNoContextForWriter(isClient bool, ext ExtParams) bool {
	if isClient {
		return ext.ClientNoContextTakeover
	}
	return ext.ServerNoContextTakeover
}

func (c *Conn) Underlying() net.Conn { return c.rwc }

func (c *Conn) ReadMessage() (Opcode, []byte, error) {
	for {
		hdr, payload, err := ReadFrame(c.br)
		if err != nil {
			return 0, nil, err
		}
		if c.isClient && hdr.Masked {
			return 0, nil, ErrMaskedFromServer
		}
		if !c.isClient && !hdr.Masked && !hdr.Opcode.IsControl() && hdr.Length > 0 {
			return 0, nil, ErrUnmaskedFromClient
		}
		asm, ready, err := c.reasm.Step(hdr, payload)
		if err != nil {
			return 0, nil, err
		}
		if !ready {
			continue
		}
		if asm.Control {
			return asm.Opcode, asm.Payload, nil
		}
		if asm.Compressed && c.inflater != nil {
			data, err := c.inflater.Inflate(asm.Payload)
			if err != nil {
				return 0, nil, err
			}
			return asm.Opcode, data, nil
		}
		return asm.Opcode, asm.Payload, nil
	}
}

func (c *Conn) WriteMessage(op Opcode, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isDead() {
		return ErrConnClosed
	}
	hdr := Header{FIN: true, Opcode: op, Masked: c.isClient, Length: uint64(len(payload))}
	if c.isClient {
		if _, err := rand.Read(hdr.MaskKey[:]); err != nil {
			return err
		}
	}
	if c.deflater != nil && (op == OpText || op == OpBinary) && len(payload) > 0 {
		compressed, err := c.deflater.Deflate(payload)
		if err != nil {
			return err
		}
		payload = compressed
		hdr.RSV1 = true
		hdr.Length = uint64(len(payload))
	}
	return WriteFrame(c.rwc, hdr, payload)
}

func (c *Conn) WriteClose(code CloseCode, reason string) error {
	return c.WriteMessage(OpClose, MakeClosePayload(code, reason))
}

func (c *Conn) Close() error {
	c.closeMu.Lock()
	if c.closed {
		c.closeMu.Unlock()
		return nil
	}
	c.closed = true
	c.closeMu.Unlock()
	if c.inflater != nil {
		_ = c.inflater.Close()
	}
	if c.deflater != nil {
		_ = c.deflater.Close()
	}
	return c.rwc.Close()
}

func (c *Conn) isDead() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closed
}
