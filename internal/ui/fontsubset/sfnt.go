package fontsubset

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

const (
	tagCmap uint32 = 0x636d6170 // 'cmap'
	tagHead uint32 = 0x68656164 // 'head'
)

type sfntTable struct {
	tag  uint32
	data []byte
}

type sfntFile struct {
	sfntVersion uint32
	tables      map[uint32]*sfntTable
}

func parseSFNT(b []byte) (*sfntFile, error) {
	if len(b) < 12 {
		return nil, errors.New("sfnt: header truncated")
	}
	version := binary.BigEndian.Uint32(b[0:4])
	switch version {
	case 0x00010000, // TrueType
		0x4F54544F, // 'OTTO' — CFF
		0x74727565: // 'true' — old TrueType
	default:
		return nil, fmt.Errorf("sfnt: unsupported version 0x%08x", version)
	}
	numTables := int(binary.BigEndian.Uint16(b[4:6]))
	if len(b) < 12+numTables*16 {
		return nil, errors.New("sfnt: table directory truncated")
	}
	f := &sfntFile{
		sfntVersion: version,
		tables:      make(map[uint32]*sfntTable, numTables),
	}
	for i := 0; i < numTables; i++ {
		off := 12 + i*16
		tag := binary.BigEndian.Uint32(b[off : off+4])
		dataOff := binary.BigEndian.Uint32(b[off+8 : off+12])
		dataLen := binary.BigEndian.Uint32(b[off+12 : off+16])
		if uint64(dataOff)+uint64(dataLen) > uint64(len(b)) {
			return nil, fmt.Errorf("sfnt: table %#x extends past EOF", tag)
		}
		data := make([]byte, dataLen)
		copy(data, b[dataOff:dataOff+dataLen])
		f.tables[tag] = &sfntTable{tag: tag, data: data}
	}
	return f, nil
}

func (f *sfntFile) serialize() ([]byte, error) {
	tags := make([]uint32, 0, len(f.tables))
	for tag := range f.tables {
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i] < tags[j] })

	numTables := uint16(len(tags))
	searchRange, entrySelector, rangeShift := dirSearchParams(numTables)

	headerSize := 12 + int(numTables)*16
	offsets := make(map[uint32]uint32, len(tags))
	pos := headerSize
	for _, tag := range tags {
		pos = (pos + 3) &^ 3
		offsets[tag] = uint32(pos)
		pos += len(f.tables[tag].data)
	}
	pos = (pos + 3) &^ 3
	out := make([]byte, pos)

	binary.BigEndian.PutUint32(out[0:4], f.sfntVersion)
	binary.BigEndian.PutUint16(out[4:6], numTables)
	binary.BigEndian.PutUint16(out[6:8], searchRange)
	binary.BigEndian.PutUint16(out[8:10], entrySelector)
	binary.BigEndian.PutUint16(out[10:12], rangeShift)

	for i, tag := range tags {
		entry := 12 + i*16
		binary.BigEndian.PutUint32(out[entry:entry+4], tag)
		// checksum filled in second pass
		binary.BigEndian.PutUint32(out[entry+8:entry+12], offsets[tag])
		binary.BigEndian.PutUint32(out[entry+12:entry+16], uint32(len(f.tables[tag].data)))
	}

	for _, tag := range tags {
		copy(out[offsets[tag]:], f.tables[tag].data)
	}

	// Per spec: zero out head.checkSumAdjustment, compute per-table checksums
	// and whole-file checksum, then set adjustment.
	headOff, headPresent := offsets[tagHead]
	if headPresent && len(f.tables[tagHead].data) >= 12 {
		binary.BigEndian.PutUint32(out[headOff+8:headOff+12], 0)
	}

	for i, tag := range tags {
		entry := 12 + i*16
		off := offsets[tag]
		length := uint32(len(f.tables[tag].data))
		checksum := tableChecksum(out[off : off+length])
		binary.BigEndian.PutUint32(out[entry+4:entry+8], checksum)
	}

	if headPresent {
		fileChecksum := tableChecksum(out)
		adjustment := uint32(0xB1B0AFBA) - fileChecksum
		binary.BigEndian.PutUint32(out[headOff+8:headOff+12], adjustment)
	}

	return out, nil
}

// tableChecksum sums b as big-endian uint32s, zero-padding the tail to
// the next 4-byte boundary if needed.
func tableChecksum(b []byte) uint32 {
	var sum uint32
	full := len(b) / 4
	for i := 0; i < full; i++ {
		sum += binary.BigEndian.Uint32(b[i*4 : i*4+4])
	}
	if rem := len(b) - full*4; rem > 0 {
		var tail [4]byte
		copy(tail[:], b[full*4:])
		sum += binary.BigEndian.Uint32(tail[:])
	}
	return sum
}

func dirSearchParams(n uint16) (searchRange, entrySelector, rangeShift uint16) {
	if n == 0 {
		return 0, 0, 0
	}
	pow := uint16(1)
	es := uint16(0)
	for pow*2 <= n {
		pow *= 2
		es++
	}
	searchRange = pow * 16
	entrySelector = es
	rangeShift = n*16 - searchRange
	return
}
