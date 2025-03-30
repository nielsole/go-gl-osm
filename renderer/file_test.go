package renderer

import (
	"os"
	"testing"
)

func TestReadMapObject(t *testing.T) {
	// Create a sample MapObject
	original := MapObject{
		BoundingBox: BoundingBox{
			Min: Point{Lat: 1.0, Lon: 2.0},
			Max: Point{Lat: 3.0, Lon: 4.0},
		},
		Points: []Point{
			{Lat: 1.5, Lon: 2.5},
			{Lat: 2.5, Lon: 3.5},
			{Lat: 1.75, Lon: 2.75},
		},
	}

	// Create a temporary file
	tmpfile, err := os.CreateTemp("", "mapobject")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpfile.Name())
	defer tmpfile.Close()

	// Write the MapObject to the file
	n, err := WriteMapObject(tmpfile, original)
	if err != nil {
		t.Fatalf("Failed to write MapObject: %v", err)
	}

	// Read the file contents into a byte slice
	tmpfile.Seek(0, 0)
	data := make([]byte, n)
	_, err = tmpfile.Read(data)
	if err != nil {
		t.Fatalf("Failed to read file contents: %v", err)
	}

	// Read the MapObject back
	var read MapObject
	err = ReadMapObject(&data, 0, &read)
	if err != nil {
		t.Fatalf("Failed to read MapObject: %v", err)
	}

	// Verify the read data matches the original
	if read.BoundingBox.Min.Lat != original.BoundingBox.Min.Lat ||
		read.BoundingBox.Min.Lon != original.BoundingBox.Min.Lon ||
		read.BoundingBox.Max.Lat != original.BoundingBox.Max.Lat ||
		read.BoundingBox.Max.Lon != original.BoundingBox.Max.Lon {
		t.Errorf("BoundingBox mismatch: got %v, want %v", read.BoundingBox, original.BoundingBox)
	}

	if len(read.Points) != len(original.Points) {
		t.Errorf("Points length mismatch: got %d, want %d", len(read.Points), len(original.Points))
	}

	for i := range original.Points {
		if read.Points[i].Lat != original.Points[i].Lat ||
			read.Points[i].Lon != original.Points[i].Lon {
			t.Errorf("Point %d mismatch: got %v, want %v", i, read.Points[i], original.Points[i])
		}
	}
}
