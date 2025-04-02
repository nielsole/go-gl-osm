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

	// Write BoundingBox - write the entire struct at once
	err := binary.Write(buf, binary.LittleEndian, mo.BoundingBox)
	if err != nil {
		return 0, err
	}

	// Write length of Points slice
	err = binary.Write(buf, binary.LittleEndian, int64(len(mo.Points)))
	if err != nil {
		return 0, err
	}

	// Write Points slice - write the entire slice at once
	err = binary.Write(buf, binary.LittleEndian, mo.Points)
	if err != nil {
		return 0, err
	}

	// Write to file
	n, err := file.Write(buf.Bytes())
	return int64(n), err
}

func ReadMapObject(mmapData *[]byte, offset int64, mo *MapObject) error {
	// Directly assign the pointer to the memory-mapped data
	mo.BoundingBox = (*BoundingBox)(unsafe.Pointer(&(*mmapData)[offset]))

	// Read the length of points (located after the BoundingBox)
	lenPoints := *(*int64)(unsafe.Pointer(&(*mmapData)[offset+32]))

	// Set up Points slice to point directly to the memory-mapped data
	pointsHeader := (*reflect.SliceHeader)(unsafe.Pointer(&mo.Points))
	pointsHeader.Data = uintptr(unsafe.Pointer(&(*mmapData)[offset+40]))
	pointsHeader.Len = int(lenPoints)
	pointsHeader.Cap = int(lenPoints)

	return nil
}
