package fontsubset

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

type cmapPair struct {
	codepoint rune
	glyphID   uint32
}

// parseUnicodeCmap walks the cmap table and returns every Unicode codepoint
// that resolves to a non-.notdef glyph. Only format 4 and format 12 Unicode
// subtables are read. Format 12 entries override format 4 entries for the
// same codepoint, since format 12 is authoritative for full Unicode.
func parseUnicodeCmap(data []byte) ([]cmapPair, error) {
	if len(data) < 4 {
		return nil, errors.New("cmap: header truncated")
	}
	numTables := int(binary.BigEndian.Uint16(data[2:4]))
	if len(data) < 4+numTables*8 {
		return nil, errors.New("cmap: encoding records truncated")
	}

	type rec struct {
		platformID, encodingID uint16
		offset                 uint32
	}
	recs := make([]rec, numTables)
	for i := 0; i < numTables; i++ {
		off := 4 + i*8
		recs[i] = rec{
			platformID: binary.BigEndian.Uint16(data[off : off+2]),
			encodingID: binary.BigEndian.Uint16(data[off+2 : off+4]),
			offset:     binary.BigEndian.Uint32(data[off+4 : off+8]),
		}
	}

	pairs := make(map[rune]uint32, 4096)

	for _, r := range recs {
		if !isUnicodeEncoding(r.platformID, r.encodingID) {
			continue
		}
		if int(r.offset)+2 > len(data) {
			continue
		}
		format := binary.BigEndian.Uint16(data[r.offset : r.offset+2])
		if format != 4 {
			continue
		}
		if err := parseFormat4(data[r.offset:], pairs); err != nil {
			return nil, fmt.Errorf("cmap fmt4 at %#x: %w", r.offset, err)
		}
	}

	for _, r := range recs {
		if !isUnicodeEncoding(r.platformID, r.encodingID) {
			continue
		}
		if int(r.offset)+2 > len(data) {
			continue
		}
		format := binary.BigEndian.Uint16(data[r.offset : r.offset+2])
		if format != 12 {
			continue
		}
		if err := parseFormat12(data[r.offset:], pairs); err != nil {
			return nil, fmt.Errorf("cmap fmt12 at %#x: %w", r.offset, err)
		}
	}

	out := make([]cmapPair, 0, len(pairs))
	for c, g := range pairs {
		if g == 0 {
			continue
		}
		out = append(out, cmapPair{codepoint: c, glyphID: g})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].codepoint < out[j].codepoint })
	return out, nil
}

func isUnicodeEncoding(platformID, encodingID uint16) bool {
	switch platformID {
	case 0: // Unicode platform — any encoding
		return true
	case 3: // Windows
		return encodingID == 1 || encodingID == 10
	}
	return false
}

func parseFormat4(sub []byte, out map[rune]uint32) error {
	if len(sub) < 14 {
		return errors.New("subtable shorter than format-4 header")
	}
	length := int(binary.BigEndian.Uint16(sub[2:4]))
	if length > len(sub) {
		return errors.New("declared length past EOF")
	}
	segCount := int(binary.BigEndian.Uint16(sub[6:8])) / 2
	if segCount == 0 {
		return nil
	}
	const headerBytes = 14
	endCodeOff := headerBytes
	startCodeOff := endCodeOff + segCount*2 + 2 // +2 for reservedPad
	idDeltaOff := startCodeOff + segCount*2
	idRangeOffsetOff := idDeltaOff + segCount*2
	if idRangeOffsetOff+segCount*2 > length {
		return errors.New("subtable arrays past declared length")
	}

	for i := 0; i < segCount; i++ {
		endCode := binary.BigEndian.Uint16(sub[endCodeOff+i*2:])
		startCode := binary.BigEndian.Uint16(sub[startCodeOff+i*2:])
		idDelta := int16(binary.BigEndian.Uint16(sub[idDeltaOff+i*2:]))
		idRangeOffset := binary.BigEndian.Uint16(sub[idRangeOffsetOff+i*2:])

		// The last segment per spec is the sentinel [0xFFFF, 0xFFFF] with
		// idDelta=1. Skip it — U+FFFF is non-character anyway.
		if startCode == 0xFFFF && endCode == 0xFFFF {
			continue
		}

		for c := uint32(startCode); c <= uint32(endCode); c++ {
			var glyph uint32
			if idRangeOffset == 0 {
				glyph = uint32(uint16(int32(c) + int32(idDelta)))
			} else {
				// Address of glyphIdArray entry, relative to the
				// idRangeOffset[i] slot itself.
				ptr := idRangeOffsetOff + i*2 + int(idRangeOffset) + int(c-uint32(startCode))*2
				if ptr+2 > length {
					continue
				}
				raw := binary.BigEndian.Uint16(sub[ptr:])
				if raw == 0 {
					// .notdef even before idDelta
					continue
				}
				glyph = uint32(uint16(int32(raw) + int32(idDelta)))
			}
			if glyph != 0 {
				out[rune(c)] = glyph
			}
		}
	}
	return nil
}

func parseFormat12(sub []byte, out map[rune]uint32) error {
	if len(sub) < 16 {
		return errors.New("subtable shorter than format-12 header")
	}
	length := binary.BigEndian.Uint32(sub[4:8])
	if uint64(length) > uint64(len(sub)) {
		return errors.New("declared length past EOF")
	}
	numGroups := binary.BigEndian.Uint32(sub[12:16])
	if 16+uint64(numGroups)*12 > uint64(length) {
		return errors.New("group array past declared length")
	}
	for i := uint32(0); i < numGroups; i++ {
		off := 16 + i*12
		startCharCode := binary.BigEndian.Uint32(sub[off : off+4])
		endCharCode := binary.BigEndian.Uint32(sub[off+4 : off+8])
		startGlyphID := binary.BigEndian.Uint32(sub[off+8 : off+12])
		for c := startCharCode; c <= endCharCode; c++ {
			out[rune(c)] = startGlyphID + (c - startCharCode)
		}
	}
	return nil
}

// buildFormat12Cmap emits a complete cmap table containing exactly one
// subtable in format 12 ("Segmented coverage"), registered under both
// Unicode platform (0, 4) and Windows full-Unicode platform (3, 10).
// Consecutive codepoints with consecutive glyph IDs are coalesced into a
// single SequentialMapGroup to minimise size.
func buildFormat12Cmap(pairs []cmapPair) []byte {
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].codepoint < pairs[j].codepoint })

	type group struct{ start, end, startGlyph uint32 }
	groups := make([]group, 0, len(pairs))
	for _, p := range pairs {
		cp := uint32(p.codepoint)
		if n := len(groups); n > 0 {
			last := &groups[n-1]
			if cp == last.end+1 && p.glyphID == last.startGlyph+(last.end-last.start)+1 {
				last.end = cp
				continue
			}
		}
		groups = append(groups, group{start: cp, end: cp, startGlyph: p.glyphID})
	}

	const (
		cmapHeader         = 4 // version + numTables
		encodingRecBytes   = 8
		fmt12HeaderBytes   = 16
		fmt12GroupBytes    = 12
		numEncodingRecords = 2
	)

	subLen := uint32(fmt12HeaderBytes + len(groups)*fmt12GroupBytes)
	subOffset := uint32(cmapHeader + numEncodingRecords*encodingRecBytes)
	totalLen := subOffset + subLen

	out := make([]byte, totalLen)

	binary.BigEndian.PutUint16(out[0:2], 0)                  // version
	binary.BigEndian.PutUint16(out[2:4], numEncodingRecords) // numTables

	// (platformID=0, encodingID=4) Unicode 2.0+ full repertoire
	binary.BigEndian.PutUint16(out[4:6], 0)
	binary.BigEndian.PutUint16(out[6:8], 4)
	binary.BigEndian.PutUint32(out[8:12], subOffset)

	// (platformID=3, encodingID=10) Windows full Unicode (UCS-4)
	binary.BigEndian.PutUint16(out[12:14], 3)
	binary.BigEndian.PutUint16(out[14:16], 10)
	binary.BigEndian.PutUint32(out[16:20], subOffset)

	// Format 12 subtable
	o := int(subOffset)
	binary.BigEndian.PutUint16(out[o:o+2], 12)                      // format
	binary.BigEndian.PutUint16(out[o+2:o+4], 0)                     // reserved
	binary.BigEndian.PutUint32(out[o+4:o+8], subLen)                // length
	binary.BigEndian.PutUint32(out[o+8:o+12], 0)                    // language
	binary.BigEndian.PutUint32(out[o+12:o+16], uint32(len(groups))) // numGroups

	for i, g := range groups {
		gOff := o + fmt12HeaderBytes + i*fmt12GroupBytes
		binary.BigEndian.PutUint32(out[gOff:gOff+4], g.start)
		binary.BigEndian.PutUint32(out[gOff+4:gOff+8], g.end)
		binary.BigEndian.PutUint32(out[gOff+8:gOff+12], g.startGlyph)
	}
	return out
}
