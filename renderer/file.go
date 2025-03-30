package renderer

import (
	"bytes"
	"encoding/binary"
	"os"
	"reflect"
	"unsafe"
)

func WriteMapObject(file *os.File, mo MapObject) (int64, error) {
	// Buffer to store binary representation
	buf := new(bytes.Buffer)

	// Write BoundingBox
	err := binary.Write(buf, binary.LittleEndian, mo.BoundingBox.Min.Lon)
	if err != nil {
		return 0, err
	}
	err = binary.Write(buf, binary.LittleEndian, mo.BoundingBox.Min.Lat)
	if err != nil {
		return 0, err
	}
	err = binary.Write(buf, binary.LittleEndian, mo.BoundingBox.Max.Lon)
	if err != nil {
		return 0, err
	}
	err = binary.Write(buf, binary.LittleEndian, mo.BoundingBox.Max.Lat)
	if err != nil {
		return 0, err
	}

	// Write length of Points slice
	err = binary.Write(buf, binary.LittleEndian, int64(len(mo.Points)))
	if err != nil {
		return 0, err
	}

	// Write Points
	for _, p := range mo.Points {
		err = binary.Write(buf, binary.LittleEndian, p.Lon)
		if err != nil {
			return 0, err
		}
		err = binary.Write(buf, binary.LittleEndian, p.Lat)
		if err != nil {
			return 0, err
		}
	}

	// Write to file
	n, err := file.Write(buf.Bytes())
	return int64(n), err
}

func ReadMapObject(mmapData *[]byte, offset int64, mo *MapObject) error {
	// Cast the memory-mapped data slice starting at offset to a BoundingBox struct
	bb := (*BoundingBox)(unsafe.Pointer(&(*mmapData)[offset]))
	mo.BoundingBox = *bb

	// Read the length of points (located after the BoundingBox)
	lenPoints := *(*int64)(unsafe.Pointer(&(*mmapData)[offset+32]))

	// Set up Points slice to point directly to the memory-mapped data
	pointsHeader := (*reflect.SliceHeader)(unsafe.Pointer(&mo.Points))
	pointsHeader.Data = uintptr(unsafe.Pointer(&(*mmapData)[offset+40]))
	pointsHeader.Len = int(lenPoints)
	pointsHeader.Cap = int(lenPoints)

	return nil
}
