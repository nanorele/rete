package workspace

import (
	"bytes"
	"runtime"
	"sync"
	"sync/atomic"
	"unsafe"
)

type jsonChunk struct {
	start int
	end   int

	inString   bool
	escapeNext bool

	fixed       int64
	indentCount int64
	indentSum   int64
	minDepth    int
	minIndent   int
	maxIndent   int
	firstIndent int
	hasFirst    bool
	depthDelta  int
	endInString bool
	endEscape   bool
	endNeed     bool

	base   int
	need   bool
	offset int64
	size   int64
}

func jsonWorkers(n int) int {
	w := runtime.GOMAXPROCS(0)
	if w > jsonMaxWorkers {
		w = jsonMaxWorkers
	}
	if max := n / jsonMinChunkSize; w > max {
		w = max
	}
	return w
}

func jsonBoundaryBack(data []byte, e, floor int) int {
	for guard := 0; guard < 64; guard++ {
		k := e - 1
		for k >= floor && (data[k] == ' ' || data[k] == '\t' || data[k] == '\n' || data[k] == '\r') {
			k--
		}
		if k < floor || (data[k] != '{' && data[k] != '[') {
			return e
		}
		e = k
	}
	return -1
}

func jsonQuoteFlips(data []byte, s, e, floor int) bool {
	flip := false
	for p := s; p < e; {
		q := bytes.IndexByte(data[p:e], '"')
		if q < 0 {
			break
		}
		pos := p + q
		k := pos - 1
		for k >= floor && data[k] == '\\' {
			k--
		}
		if (pos-1-k)&1 == 0 {
			flip = !flip
		}
		p = pos + 1
	}
	return flip
}

func jsonEscapePending(data []byte, at, floor int) bool {
	k := at - 1
	for k >= floor && data[k] == '\\' {
		k--
	}
	return (at-1-k)&1 == 1
}

func measureJSONChunk(data []byte, c *jsonChunk) {
	seg := data[c.start:c.end]
	n := len(seg)
	inStr, esc, need := c.inString, c.escapeNext, false
	depth, minDepth, minInd, maxInd := 0, 0, 0, 0
	first, hasFirst := 0, false
	var fixed, cnt, sum int64

	markIndent := func() {
		cnt++
		sum += int64(depth)
		if depth < minInd {
			minInd = depth
		}
		if depth > maxInd {
			maxInd = depth
		}
	}

	i := 0
	for i < n {
		if inStr {
			j := i
			if esc {
				esc = false
				j++
			}
			for {
				if j >= n {
					fixed += int64(n - i)
					i = n
					break
				}
				q := bytes.IndexByte(seg[j:], '"')
				end := n
				if q >= 0 {
					end = j + q
				}
				if bs := bytes.IndexByte(seg[j:end], '\\'); bs >= 0 {
					j += bs + 2
					if j > n {
						esc = true
						fixed += int64(n - i)
						i = n
						break
					}
					continue
				}
				if q < 0 {
					fixed += int64(n - i)
					i = n
					break
				}
				fixed += int64(end + 1 - i)
				i = end + 1
				inStr = false
				break
			}
			continue
		}

		b := seg[i]
		i++
		switch b {
		case '"':
			if !hasFirst {
				first, hasFirst = depth, true
			}
			if need {
				markIndent()
				need = false
			}
			fixed++
			inStr = true
		case '{', '[':
			if !hasFirst {
				first, hasFirst = depth, true
			}
			if need {
				markIndent()
				need = false
			}
			fixed++
			j := i
			for j < n && (seg[j] == ' ' || seg[j] == '\t' || seg[j] == '\n' || seg[j] == '\r') {
				j++
			}
			if j < n && ((b == '{' && seg[j] == '}') || (b == '[' && seg[j] == ']')) {
				fixed++
				i = j + 1
				continue
			}
			depth++
			need = true
		case '}', ']':
			depth--
			if depth < minDepth {
				minDepth = depth
			}
			markIndent()
			fixed++
		case ',':
			fixed++
			need = true
		case ':':
			fixed += 2
		case ' ', '\t', '\n', '\r':
		default:
			if !hasFirst {
				first, hasFirst = depth, true
			}
			if need {
				markIndent()
				need = false
			}
			start := i - 1
			for i < n && !jsonTokenEnd[seg[i]] {
				i++
			}
			fixed += int64(i - start)
		}
	}

	c.fixed = fixed
	c.indentCount = cnt
	c.indentSum = sum
	c.minDepth = minDepth
	c.minIndent = minInd
	c.maxIndent = maxInd
	c.firstIndent = first
	c.hasFirst = hasFirst
	c.depthDelta = depth
	c.endInString = inStr
	c.endEscape = esc
	c.endNeed = need
}

func jsonRunParallel(n int, fn func(int)) {
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			fn(i)
		}(i)
	}
	wg.Wait()
}

func formatJSONParallel(data []byte, state *JSONFormatterState) (string, bool) {
	if state.Indent < 0 || state.Indent > maxIndentDepth {
		return "", false
	}
	w := jsonWorkers(len(data))
	if w < 2 {
		return "", false
	}

	floor := 0
	if state.EscapeNext {
		floor = 1
	}

	chunks := make([]jsonChunk, 0, w)
	span := len(data) / w
	prev := 0
	for i := 1; i <= w; i++ {
		e := len(data)
		if i < w {
			e = jsonBoundaryBack(data, i*span, floor)
			if e < 0 {
				return "", false
			}
		}
		if e <= prev {
			continue
		}
		chunks = append(chunks, jsonChunk{start: prev, end: e})
		prev = e
	}
	if len(chunks) < 2 {
		return "", false
	}

	flips := make([]bool, len(chunks))
	jsonRunParallel(len(chunks), func(i int) {
		flips[i] = jsonQuoteFlips(data, chunks[i].start, chunks[i].end, floor)
	})

	inStr := state.InString
	esc := state.EscapeNext
	for i := range chunks {
		chunks[i].inString = inStr
		chunks[i].escapeNext = esc
		if flips[i] {
			inStr = !inStr
		}
		esc = false
		if i+1 < len(chunks) && inStr {
			esc = jsonEscapePending(data, chunks[i+1].start, floor)
		}
	}

	jsonRunParallel(len(chunks), func(i int) { measureJSONChunk(data, &chunks[i]) })

	base := state.Indent
	need := state.NeedIndent
	var total int64
	for i := range chunks {
		c := &chunks[i]
		if i > 0 {
			p := &chunks[i-1]
			if c.inString != p.endInString || c.escapeNext != p.endEscape {
				return "", false
			}
		}
		if base+c.minDepth < 0 || base+c.minIndent < 0 || base+c.maxIndent > maxIndentDepth {
			return "", false
		}
		c.base = base
		c.need = need
		c.offset = total
		c.size = c.fixed + c.indentCount + 2*c.indentSum + 2*int64(base)*c.indentCount
		if need && c.hasFirst {
			d := base + c.firstIndent
			if d < 0 || d > maxIndentDepth {
				return "", false
			}
			c.size += int64(1 + 2*d)
		}
		total += c.size
		base += c.depthDelta
		need = c.endNeed || (need && !c.hasFirst)
	}
	if total == 0 {
		return "", false
	}

	buf := make([]byte, total)
	var failed atomic.Bool
	jsonRunParallel(len(chunks), func(i int) {
		c := &chunks[i]
		st := JSONFormatterState{
			Indent:     c.base,
			InString:   c.inString,
			NeedIndent: c.need,
			EscapeNext: c.escapeNext,
		}
		dst := buf[c.offset : c.offset : c.offset+c.size]
		out := appendFormatJSON(dst, data[c.start:c.end], &st)
		if int64(len(out)) != c.size {
			failed.Store(true)
			return
		}
		if c.size > 0 && &out[0] != &buf[c.offset] {
			copy(buf[c.offset:], out)
		}
	})
	if failed.Load() {
		return "", false
	}

	last := &chunks[len(chunks)-1]
	state.Indent = base
	state.NeedIndent = need
	state.InString = last.endInString
	state.EscapeNext = last.endEscape
	return unsafe.String(unsafe.SliceData(buf), len(buf)), true
}
