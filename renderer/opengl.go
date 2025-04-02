package renderer

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"runtime"
	"strconv"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/nielsole/go-gl-osm/utils"
)

const (
	vertexShaderSource = `
		#version 410
		in vec2 position;
		uniform mat4 projection;
		void main() {
			gl_Position = projection * vec4(position, 0.0, 1.0);
		}
	` + "\x00"

	fragmentShaderSource = `
		#version 410
		out vec4 frag_color;
		void main() {
			// Black lines with full opacity
			frag_color = vec4(0.0, 0.0, 0.0, 1.0);
		}
	` + "\x00"
)

var (
	program     uint32
	vao         uint32
	vbo         uint32
	fbo         uint32
	texture     uint32
	projUniform int32
)

var renderChan = make(chan renderRequest)

type renderRequest struct {
	w            http.ResponseWriter
	r            *http.Request
	data         *Data
	maxTreeDepth uint32
	mmapData     *[]byte
	done         chan struct{}
	ctx          context.Context
}

func HandleRenderRequestOpenGL(w http.ResponseWriter, r *http.Request, data *Data, maxTreeDepth uint32, mmapData *[]byte) {
	done := make(chan struct{})
	renderChan <- renderRequest{w, r, data, maxTreeDepth, mmapData, done, r.Context()}
	<-done
}

// func init() {
// 	runtime.LockOSThread()
// 	var err error
// 	mainRenderer, err = NewOpenGLRenderer()
// 	if err != nil {
// 		log.Fatalf("Failed to initialize OpenGL renderer: %v", err)
// 	}
// }

func RenderLoop(ctx context.Context, mainRenderer *OpenGLRenderer) {
	runtime.LockOSThread()
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-renderChan:
			select {
			case <-req.ctx.Done():
				close(req.done)
				continue
			default:
				mainRenderer.renderLock.Lock()
				mainRenderer.window.MakeContextCurrent()
				handleRenderRequest(req.w, req.r, req.data, req.maxTreeDepth, req.mmapData, mainRenderer)
				mainRenderer.renderLock.Unlock()
				close(req.done)
			}
		}
	}
}

func handleRenderRequest(w http.ResponseWriter, r *http.Request, data *Data, maxTreeDepth uint32, mmapData *[]byte, mainRenderer *OpenGLRenderer) {
	z, x, y, ext, err := utils.ParsePath(r.URL.Path)
	if ext != "png" {
		http.Error(w, "Only png is supported", http.StatusBadRequest)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	const S = 256
	// Use the global renderer's verticesBuffer
	vertices := mainRenderer.prepareTileVertices(data, mmapData, x, y, z)
	dataBytes := drawOffscreen(vertices, S)
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(dataBytes)))
	w.Write(dataBytes)
}

func checkShaderError(shader uint32) error {
	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		log := make([]byte, logLength)
		gl.GetShaderInfoLog(shader, logLength, nil, &log[0])
		return fmt.Errorf("shader compilation failed: %s", string(log))
	}
	return nil
}

func Run(ctx context.Context) error {
	// Initialize OpenGL and run render loop on main thread
	runtime.LockOSThread()

	var err error
	mainRenderer, err := NewOpenGLRenderer()
	if err != nil {
		return fmt.Errorf("failed to initialize OpenGL renderer: %v", err)
	}
	defer mainRenderer.Close()

	RenderLoop(ctx, mainRenderer)
	return nil
}

func checkGLError(prefix string) error {
	err := gl.GetError()
	if err != 0 {
		return fmt.Errorf("%s: OpenGL error: %d", prefix, err)
	}
	return nil
}

func drawOffscreen(vertices []float32, size int32) []byte {
	// Add check for empty vertices at the start
	if len(vertices) == 0 {
		// Return a blank white tile
		img := image.NewRGBA(image.Rect(0, 0, int(size), int(size)))
		for y := 0; y < int(size); y++ {
			for x := 0; x < int(size); x++ {
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
		var buf bytes.Buffer
		png.Encode(&buf, img)
		return buf.Bytes()
	}

	// Set up framebuffer with texture
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	if err := checkGLError("BindFramebuffer"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Bind and resize texture
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, size, size, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	if err := checkGLError("TexImage2D"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Attach texture to framebuffer
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, texture, 0)
	if err := checkGLError("FramebufferTexture2D"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Check framebuffer status
	status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER)
	if err := checkGLError("CheckFramebufferStatus"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	if status != gl.FRAMEBUFFER_COMPLETE {
		log.Printf("Framebuffer is not complete. Status: %d", status)
		switch status {
		case gl.FRAMEBUFFER_UNDEFINED:
			log.Printf("FRAMEBUFFER_UNDEFINED")
		case gl.FRAMEBUFFER_INCOMPLETE_ATTACHMENT:
			log.Printf("FRAMEBUFFER_INCOMPLETE_ATTACHMENT")
		case gl.FRAMEBUFFER_INCOMPLETE_MISSING_ATTACHMENT:
			log.Printf("FRAMEBUFFER_INCOMPLETE_MISSING_ATTACHMENT")
		case gl.FRAMEBUFFER_INCOMPLETE_DRAW_BUFFER:
			log.Printf("FRAMEBUFFER_INCOMPLETE_DRAW_BUFFER")
		case gl.FRAMEBUFFER_INCOMPLETE_READ_BUFFER:
			log.Printf("FRAMEBUFFER_INCOMPLETE_READ_BUFFER")
		case gl.FRAMEBUFFER_UNSUPPORTED:
			log.Printf("FRAMEBUFFER_UNSUPPORTED")
		case gl.FRAMEBUFFER_INCOMPLETE_MULTISAMPLE:
			log.Printf("FRAMEBUFFER_INCOMPLETE_MULTISAMPLE")
		case gl.FRAMEBUFFER_INCOMPLETE_LAYER_TARGETS:
			log.Printf("FRAMEBUFFER_INCOMPLETE_LAYER_TARGETS")
		}
	}

	// Clear and set viewport
	gl.Viewport(0, 0, size, size)
	gl.ClearColor(1.0, 1.0, 1.0, 1.0) // Set alpha to 1.0 for opaque white
	gl.Clear(gl.COLOR_BUFFER_BIT)     // Only clear color buffer since we're doing 2D rendering

	if err := checkGLError("Clear"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Use shader program
	gl.UseProgram(program)
	if err := checkGLError("UseProgram"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Set up orthographic projection
	var projection [16]float32
	projection[0] = 2.0 / float32(size)
	projection[5] = -2.0 / float32(size)
	projection[10] = 1
	projection[12] = -1
	projection[13] = 1
	projection[15] = 1
	gl.UniformMatrix4fv(projUniform, 1, false, &projection[0])
	if err := checkGLError("UniformMatrix"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Bind and update vertex buffer
	gl.BindVertexArray(vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)
	if err := checkGLError("Buffer setup"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Set up vertex attributes
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 0, nil)
	if err := checkGLError("Vertex attributes"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Draw lines with proper alpha blending
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)
	gl.DrawArrays(gl.LINES, 0, int32(len(vertices)/2))
	gl.Disable(gl.BLEND)
	if err := checkGLError("DrawArrays"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Read pixels
	pixels := make([]byte, size*size*4)
	gl.ReadPixels(0, 0, size, size, gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))
	if err := checkGLError("ReadPixels"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Convert to image
	img := image.NewRGBA(image.Rect(0, 0, int(size), int(size)))
	for y := 0; y < int(size); y++ {
		for x := 0; x < int(size); x++ {
			i := (y*int(size) + x) * 4
			img.Set(x, int(size)-y-1, color.RGBA{
				R: pixels[i],
				G: pixels[i+1],
				B: pixels[i+2],
				A: pixels[i+3],
			})
		}
	}

	// Reset framebuffer
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)

	// Encode to PNG
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func HandleOpenGLRenderRequest(w http.ResponseWriter, r *http.Request, data *Data, maxZoom int, mmapData *[]byte, renderer *OpenGLRenderer) {
	// Extract x, y, z from request path
	x, y, z, _, err := utils.ParsePath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Render tile using OpenGL
	img := renderer.RenderTile(data, mmapData, x, y, z)

	// Write PNG response
	w.Header().Set("Content-Type", "image/png")
	png.Encode(w, img)
}
