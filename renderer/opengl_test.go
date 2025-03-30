//go:build !windows && !darwin
// +build !windows,!darwin

package renderer

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"syscall"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	sharedWindow *glfw.Window
	initialized  bool
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

func BenchmarkServeEmptyTileOpenGL(b *testing.B) {
	b.StopTimer()
	pathTile := "/tile/11/1086/664.png"
	tempFile, err := ioutil.TempFile("", "example")
	if err != nil {
		fmt.Println("Cannot create temp file:", err)
		os.Exit(1)
	}
	defer os.Remove(tempFile.Name())
	data, err := LoadData("./prepared.osm.pbf", 15, tempFile)
	if err != nil {
		b.Error(err)
	}
	tempFileName := tempFile.Name()
	tempFile.Close()

	// Memory-map the file
	mmapData, mmapFile, err := Mmap(tempFileName)
	if err != nil {
		log.Fatalf("There was an error memory-mapping temp file: %v", err)
	}
	defer syscall.Munmap(*mmapData)
	defer mmapFile.Close()

	// Initialize OpenGL context
	renderer, err := NewOpenGLRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer renderer.Close()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", pathTile, bytes.NewReader([]byte{}))
		w := httptest.NewRecorder()
		HandleOpenGLRenderRequest(w, req, data, 15, mmapData, renderer)

		// Ensure the response was written
		result := w.Result()
		if result.StatusCode != http.StatusOK {
			b.Fatalf("Expected status code %d but got %d", http.StatusOK, result.StatusCode)
		}
		// Read and close the body to ensure everything was written
		body, err := io.ReadAll(result.Body)
		result.Body.Close()
		if err != nil {
			b.Fatal(err)
		}
		if len(body) == 0 {
			b.Fatal("Expected non-empty response body")
		}
	}
}

func setupOpenGL() error {
	if initialized {
		return nil
	}
	runtime.LockOSThread()

	if err := glfw.Init(); err != nil {
		return fmt.Errorf("failed to initialize GLFW: %v", err)
	}

	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Visible, glfw.False)

	var err error
	sharedWindow, err = glfw.CreateWindow(256, 256, "", nil, nil)
	if err != nil {
		glfw.Terminate()
		return fmt.Errorf("failed to create window: %v", err)
	}

	sharedWindow.MakeContextCurrent()
	initialized = true
	return nil
}

func BenchmarkServeFullTileOpenGL(b *testing.B) {
	b.StopTimer()

	if err := setupOpenGL(); err != nil {
		b.Fatal(err)
	}

	pathTile := "/tile/11/1081/661.png"
	tempFile, err := ioutil.TempFile("", "example")
	if err != nil {
		b.Fatal("Cannot create temp file:", err)
	}
	defer os.Remove(tempFile.Name())

	// Load the data first
	data, err := LoadData("../prepared.osm.pbf", 15, tempFile)
	if err != nil {
		b.Fatal(err)
	}
	tempFileName := tempFile.Name()
	tempFile.Close()

	// Memory-map the file
	mmapData, mmapFile, err := Mmap(tempFileName)
	if err != nil {
		b.Fatal("Error memory-mapping temp file:", err)
	}
	defer syscall.Munmap(*mmapData)
	defer mmapFile.Close()

	// Initialize OpenGL context
	renderer, err := NewOpenGLRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer renderer.Close()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", pathTile, bytes.NewReader([]byte{}))
		w := httptest.NewRecorder()
		HandleOpenGLRenderRequest(w, req, data, 15, mmapData, renderer)

		// Ensure the response was written
		result := w.Result()
		if result.StatusCode != http.StatusOK {
			b.Fatalf("Expected status code %d but got %d", http.StatusOK, result.StatusCode)
		}
		// Read and close the body to ensure everything was written
		body, err := io.ReadAll(result.Body)
		result.Body.Close()
		if err != nil {
			b.Fatal(err)
		}
		if len(body) == 0 {
			b.Fatal("Expected non-empty response body")
		}
	}
}

func BenchmarkServeFullTileZ3OpenGL(b *testing.B) {
	b.StopTimer()
	pathTile := "/tile/3/4/2.png"
	tempFile, err := ioutil.TempFile("", "example")
	if err != nil {
		fmt.Println("Cannot create temp file:", err)
		os.Exit(1)
	}
	defer os.Remove(tempFile.Name())
	data, err := LoadData("./prepared.osm.pbf", 15, tempFile)
	if err != nil {
		b.Error(err)
	}
	tempFileName := tempFile.Name()
	tempFile.Close()

	// Memory-map the file
	mmapData, mmapFile, err := Mmap(tempFileName)
	if err != nil {
		log.Fatalf("There was an error memory-mapping temp file: %v", err)
	}
	defer syscall.Munmap(*mmapData)
	defer mmapFile.Close()

	// Initialize OpenGL context
	renderer, err := NewOpenGLRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer renderer.Close()

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", pathTile, bytes.NewReader([]byte{}))
		w := httptest.NewRecorder()
		HandleOpenGLRenderRequest(w, req, data, 15, mmapData, renderer)

		// Ensure the response was written
		result := w.Result()
		if result.StatusCode != http.StatusOK {
			b.Fatalf("Expected status code %d but got %d", http.StatusOK, result.StatusCode)
		}
		// Read and close the body to ensure everything was written
		body, err := io.ReadAll(result.Body)
		result.Body.Close()
		if err != nil {
			b.Fatal(err)
		}
		if len(body) == 0 {
			b.Fatal("Expected non-empty response body")
		}
	}
}
