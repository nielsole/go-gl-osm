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
	runDrawOffscreenTest(t, false)
}

func TestDrawOffscreenTransparency(t *testing.T) {
	runDrawOffscreenTest(t, true)
}

func runDrawOffscreenTest(t *testing.T, checkTransparency bool) {
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

	// Count pixels
	bounds := img.Bounds()
	nonWhiteCount := 0
	transparentCount := 0
	totalPixels := bounds.Dx() * bounds.Dy()

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			// In 16-bit color space (values from 0 to 65535)
			if a == 0 {
				transparentCount++
			}
			// Check if pixel is not white (ignoring alpha)
			if r != 65535 || g != 65535 || b != 65535 {
				nonWhiteCount++
			}
		}
	}

	// We should have at least some non-white pixels for our lines
	if nonWhiteCount == 0 {
		t.Error("Expected some non-white pixels for drawn lines, but image appears to be blank")
	}

	if checkTransparency {
		// The image should be fully opaque
		if transparentCount > 0 {
			t.Errorf("Expected opaque image, but found %d transparent pixels out of %d total pixels", transparentCount, totalPixels)
		}

		// Background should be white
		whiteCount := totalPixels - nonWhiteCount
		if whiteCount < totalPixels*90/100 { // At least 90% should be white background
			t.Errorf("Expected mostly white background, but only %d out of %d pixels are white", whiteCount, totalPixels)
		}
	}
}
