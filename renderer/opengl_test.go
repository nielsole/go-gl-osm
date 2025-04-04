//go:build !windows && !darwin
// +build !windows,!darwin

package renderer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"sync"
	"syscall"
	"testing"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	sharedWindow *glfw.Window
	initialized  bool
	testLock     sync.Mutex
)

func TestMain(m *testing.M) {
	// GLFW event handling must run on the main thread
	runtime.LockOSThread()

	// Initialize GLFW for all tests
	if err := glfw.Init(); err != nil {
		log.Fatalf("Failed to initialize GLFW: %v", err)
	}
	defer glfw.Terminate()

	// Run all tests
	code := m.Run()
	os.Exit(code)
}

func TestDrawOffscreen(t *testing.T) {
	runDrawOffscreenTest(t, false)
}

func TestDrawOffscreenTransparency(t *testing.T) {
	runDrawOffscreenTest(t, true)
}

func runDrawOffscreenTest(t *testing.T, checkTransparency bool) {
	testLock.Lock()
	defer testLock.Unlock()

	runtime.LockOSThread()

	// Initialize OpenGL using the same code as production
	renderer, err := NewOpenGLRenderer()
	if err != nil {
		t.Fatalf("Failed to initialize OpenGL: %v", err)
	}
	defer func() {
		if renderer != nil && renderer.window != nil {
			renderer.Close()
		}
	}()

	// Create test vertices for a diagonal line across the image
	vertices := []float32{
		0, 0, 0, // Tile coordinates (x, y, z)
		-180.0, -85.0, // Bottom-left of world
		180.0, 85.0, // Top-right of world
		-180.0, 85.0, // Top-left of world
		180.0, -85.0, // Bottom-right of world
	}

	// Render the image
	const size int32 = 256
	img := drawOffscreen(vertices, size)

	// Decode the PNG
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
	testLock.Lock()
	defer testLock.Unlock()

	runtime.LockOSThread()

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

	renderer, err := NewOpenGLRenderer()
	if err != nil {
		b.Fatal(err)
	}
	defer func() {
		if renderer != nil && renderer.window != nil {
			renderer.Close()
		}
	}()

	b.ResetTimer()
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

// db8aea62f12c1c49517be2f1fff72de808df7b06
// cpu: AMD Ryzen 7 5700G with Radeon Graphics
// BenchmarkServeFullTileOpenGL-16              177          99015424 ns/op

// 2307d92df2000084e5133d42a7df062520717043
// cpu: AMD Ryzen 7 5700G with Radeon Graphics
// BenchmarkServeFullTileOpenGL-16              724          22806541 ns/op

// f17f34cc40e064a18e54ea20b994fedadfdde9fb
// cpu: AMD Ryzen 7 5700G with Radeon Graphics
// BenchmarkServeFullTileOpenGL-16              1299         13154137 ns/op

func BenchmarkServeFullTileOpenGL(b *testing.B) {
	testLock.Lock()
	defer testLock.Unlock()

	runtime.LockOSThread()

	// Load test data
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

	renderer, err := NewOpenGLRenderer()
	if err != nil {
		b.Fatalf("failed to create renderer: %v", err)
	}
	defer func() {
		if renderer != nil && renderer.window != nil {
			renderer.Close()
		}
	}()

	// Create a context for the benchmark
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the render loop in a separate goroutine
	go RenderLoop(ctx, renderer)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", pathTile, bytes.NewReader([]byte{}))
		w := httptest.NewRecorder()
		HandleRenderRequestOpenGL(w, req, data, 15, mmapData)

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

	// Initialize OpenGL using the same code as production
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
