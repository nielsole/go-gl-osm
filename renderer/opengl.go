package renderer

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"strconv"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

func HandleRenderRequestOpenGL(w http.ResponseWriter, r *http.Request, data *Data, maxTreeDepth uint32, mmapData *[]byte) {
	dataBytes := drawOffscreen()
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(dataBytes)))
	w.Write(dataBytes)

}

func InitOpenGL() {
	if err := glfw.Init(); err != nil {
		log.Fatalf("failed to initialize glfw: %v", err)
	}
	glfw.WindowHint(glfw.Visible, glfw.False)    // hidden window
	glfw.WindowHint(glfw.ContextVersionMajor, 4) // targeting OpenGL version 4.1
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	window, err := glfw.CreateWindow(256, 256, "", nil, nil)
	if err != nil {
		log.Fatalf("failed to create window: %v", err)
	}
	window.MakeContextCurrent()

	if err := gl.Init(); err != nil {
		log.Fatalf("failed to initialize go-gl: %v", err)
	}
}

func CleanupOpenGL() {
	glfw.Terminate()
}

func drawOffscreen() []byte {
	// OpenGL drawing commands go here
	// For simplicity, this example will not include actual OpenGL drawing commands,
	// but you would use OpenGL to draw your rectangle to an FBO here.

	// Create a placeholder image instead of actual OpenGL drawing for demonstration
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	red := color.RGBA{255, 0, 0, 255}
	for x := 0; x < 100; x++ {
		for y := 0; y < 100; y++ {
			img.Set(x, y, red)
		}
	}

	var buf bytes.Buffer
	png.Encode(&buf, img) // Encode the image to PNG
	return buf.Bytes()
}
