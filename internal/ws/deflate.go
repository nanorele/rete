package ws

import (
	"bytes"
	"compress/flate"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const flateHistorySize = 32 * 1024

type ExtParams struct {
	Negotiated              bool
	ServerNoContextTakeover bool
	ClientNoContextTakeover bool
	ServerMaxWindowBits     int
	ClientMaxWindowBits     int
}

func ParseExtensions(header string) ExtParams {
	var p ExtParams
	if header == "" {
		return p
	}
	for ext := range strings.SplitSeq(header, ",") {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		parts := strings.Split(ext, ";")
		name := strings.TrimSpace(parts[0])
		if !strings.EqualFold(name, "permessage-deflate") {
			continue
		}
		p.Negotiated = true
		for _, kv := range parts[1:] {
			kv = strings.TrimSpace(kv)
			eq := strings.IndexByte(kv, '=')
			key := kv
			val := ""
			if eq >= 0 {
				key = strings.TrimSpace(kv[:eq])
				val = strings.TrimSpace(strings.Trim(kv[eq+1:], `"`))
			}
			switch strings.ToLower(key) {
			case "server_no_context_takeover":
				p.ServerNoContextTakeover = true
			case "client_no_context_takeover":
				p.ClientNoContextTakeover = true
			case "server_max_window_bits":
				if v, err := strconv.Atoi(val); err == nil {
					p.ServerMaxWindowBits = v
				}
			case "client_max_window_bits":
				if v, err := strconv.Atoi(val); err == nil {
					p.ClientMaxWindowBits = v
				}
			}
		}
		return p
	}
	return p
}

func OfferExtensions() string {
	return "permessage-deflate; client_max_window_bits"
}

var syncTail = [...]byte{0x00, 0x00, 0xff, 0xff}
var finalEmptyBlock = [...]byte{0x01, 0x00, 0x00, 0xff, 0xff}

type Inflater struct {
	noContext bool
	mu        sync.Mutex
	history   []byte
}

func NewInflater(noContextTakeover bool) *Inflater {
	return &Inflater{noContext: noContextTakeover}
}

func (i *Inflater) Inflate(payload []byte) ([]byte, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	src := io.MultiReader(
		bytes.NewReader(payload),
		bytes.NewReader(syncTail[:]),
		bytes.NewReader(finalEmptyBlock[:]),
	)
	var fr io.ReadCloser
	if i.noContext || len(i.history) == 0 {
		fr = flate.NewReader(src)
	} else {
		fr = flate.NewReaderDict(src, i.history)
	}
	defer fr.Close()
	out, err := io.ReadAll(fr)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if !i.noContext {
		i.history = appendHistory(i.history, out)
	}
	return out, nil
}

func (i *Inflater) Close() error { return nil }

type Deflater struct {
	noContext bool
	mu        sync.Mutex
	buf       *bytes.Buffer
	history   []byte
}

func NewDeflater(noContextTakeover bool) (*Deflater, error) {
	return &Deflater{noContext: noContextTakeover, buf: &bytes.Buffer{}}, nil
}

func (d *Deflater) Deflate(payload []byte) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.buf.Reset()
	var fw *flate.Writer
	var err error
	if d.noContext || len(d.history) == 0 {
		fw, err = flate.NewWriter(d.buf, flate.DefaultCompression)
	} else {
		fw, err = flate.NewWriterDict(d.buf, flate.DefaultCompression, d.history)
	}
	if err != nil {
		return nil, err
	}
	if _, err := fw.Write(payload); err != nil {
		return nil, err
	}
	if err := fw.Flush(); err != nil {
		return nil, err
	}
	out := d.buf.Bytes()
	if len(out) >= 4 && bytes.Equal(out[len(out)-4:], syncTail[:]) {
		out = out[:len(out)-4]
	}
	result := slices.Clone(out)
	if !d.noContext {
		d.history = appendHistory(d.history, payload)
	}
	return result, nil
}

func (d *Deflater) Close() error { return nil }

func appendHistory(history, fresh []byte) []byte {
	if len(fresh) >= flateHistorySize {
		return slices.Clone(fresh[len(fresh)-flateHistorySize:])
	}
	history = append(history, fresh...)
	if len(history) > flateHistorySize {
		history = history[len(history)-flateHistorySize:]
	}
	return history
}
