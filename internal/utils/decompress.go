package utils

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
)

type zstdReadCloser struct{ *zstd.Decoder }

func (z zstdReadCloser) Close() error {
	z.Decoder.Close()
	return nil
}

type multiCloser struct {
	io.Reader
	closers []io.Closer
}

func (m *multiCloser) Close() error {
	var firstErr error
	for i := len(m.closers) - 1; i >= 0; i-- {
		if err := m.closers[i].Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func DecompressBody(resp *http.Response) io.ReadCloser {
	if resp == nil {
		return nil
	}
	if resp.Body == nil {
		return nil
	}
	if resp.Uncompressed {
		return resp.Body
	}
	enc := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding")))
	if enc == "" || enc == "identity" {
		return resp.Body
	}
	parts := strings.Split(enc, ",")
	var reader io.Reader = resp.Body
	closers := []io.Closer{resp.Body}
	for i := len(parts) - 1; i >= 0; i-- {
		e := strings.TrimSpace(parts[i])
		switch e {
		case "", "identity":
			continue
		case "gzip", "x-gzip":
			gz, err := gzip.NewReader(reader)
			if err != nil {
				return resp.Body
			}
			reader = gz
			closers = append(closers, gz)
		case "deflate":
			br := bufio.NewReader(reader)
			hdr, _ := br.Peek(2)
			if len(hdr) >= 2 && hdr[0] == 0x78 {
				zr, err := zlib.NewReader(br)
				if err != nil {
					return resp.Body
				}
				reader = zr
				closers = append(closers, zr)
			} else {
				fr := flate.NewReader(br)
				reader = fr
				closers = append(closers, fr)
			}
		case "br":
			reader = brotli.NewReader(reader)
		case "zstd":
			zr, err := zstd.NewReader(reader)
			if err != nil {
				return resp.Body
			}
			reader = zr
			closers = append(closers, zstdReadCloser{zr})
		default:
			return resp.Body
		}
	}
	return &multiCloser{Reader: reader, closers: closers}
}

func TrimTrailingWhitespace(s string) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t\r")
	}
	return strings.Join(lines, "\n")
}
