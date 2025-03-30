package renderer

import (
	"bytes"
	"image/png"
	"runtime"
	"testing"
)

func init() {
	// GLFW event handling must run on the main thread
	runtime.LockOSThread()
}

func TestDrawOffscreen(t *testing.T) {
	// Initialize OpenGL
	InitOpenGL()
	defer CleanupOpenGL()

	// Create test vertices for a diagonal line across the image
	vertices := []float32{
		0, 0, // Start point
		256, 256, // End point
		0, 256, // Another line
		256, 0, // Creating an X shape
	}

	// Render the image
	const size int32 = 256
	data := drawOffscreen(vertices, size)

	// Decode the PNG
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode PNG: %v", err)
	}

	// Count non-white pixels
	bounds := img.Bounds()
	nonWhiteCount := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			// Check if pixel is not white (white is 65535 in this color space)
			if r != 65535 || g != 65535 || b != 65535 {
				nonWhiteCount++
			}
		}
	}

	// We should have at least some non-white pixels for our lines
	if nonWhiteCount == 0 {
		t.Error("Expected some non-white pixels for drawn lines, but image appears to be blank")
	}

	// Optionally save the test image for visual inspection
	// Uncomment for debugging:
	// os.WriteFile("test_output.png", data, 0644)
}
