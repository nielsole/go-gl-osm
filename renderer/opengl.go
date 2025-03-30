package renderer

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"strconv"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
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
			frag_color = vec4(0.0, 0.0, 0.0, 1.0);
		}
	` + "\x00"
)

var (
	program     uint32
	vao         uint32
	vbo         uint32
	fbo         uint32
	rbo         uint32
	projUniform int32
)

func HandleRenderRequestOpenGL(w http.ResponseWriter, r *http.Request, data *Data, maxTreeDepth uint32, mmapData *[]byte) {
	z, x, y, ext, err := utils.ParsePath(r.URL.Path)
	if ext != "png" {
		http.Error(w, "Only png is supported", http.StatusBadRequest)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	tile := Tile{
		X: x,
		Y: y,
		Z: z,
	}
	bbox := getBoundingBox(tile)
	// fmt.Printf("Bounding box is from Lat %f, Lon %f to Lat %f, Lon %f\n", bbox.Min.Lat, bbox.Min.Lon, bbox.Max.Lat, bbox.Max.Lon)

	const S = 256
	parentTile := tile
	for tempZ := z; tempZ > maxTreeDepth; tempZ-- {
		parentTile = parentTile.getParent()
	}
	wayIndices, ok := data.Tiles[parentTile.index()]
	if !ok {
		// Return 404
		w.WriteHeader(http.StatusNotFound)
		return
	}
	way := MapObject{Points: make([]Point, 0, data.MaxPoints)}
	var vertices []float32
	for _, wayReference := range *wayIndices {
		err := ReadMapObject(mmapData, int64(wayReference), &way)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if !bbox.overlaps(way.BoundingBox) {
			continue
		}

		for i := 0; i < len(way.Points)-1; i++ {
			p1 := pointToPixels(way.Points[i], bbox, S)
			p2 := pointToPixels(way.Points[i+1], bbox, S)

			// Add line vertices
			vertices = append(vertices,
				float32(p1.X), float32(p1.Y),
				float32(p2.X), float32(p2.Y))
		}
	}

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

	// Initialize shaders and program
	vertexShader := gl.CreateShader(gl.VERTEX_SHADER)
	vertexShaderCString, free := gl.Strs(vertexShaderSource)
	gl.ShaderSource(vertexShader, 1, vertexShaderCString, nil)
	free()
	gl.CompileShader(vertexShader)
	if err := checkShaderError(vertexShader); err != nil {
		log.Fatalf("vertex shader error: %v", err)
	}

	fragmentShader := gl.CreateShader(gl.FRAGMENT_SHADER)
	fragmentShaderCString, free := gl.Strs(fragmentShaderSource)
	gl.ShaderSource(fragmentShader, 1, fragmentShaderCString, nil)
	free()
	gl.CompileShader(fragmentShader)
	if err := checkShaderError(fragmentShader); err != nil {
		log.Fatalf("fragment shader error: %v", err)
	}

	program = gl.CreateProgram()
	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		logMsg := make([]byte, logLength)
		gl.GetProgramInfoLog(program, logLength, nil, &logMsg[0])
		log.Fatalf("program link error: %s", string(logMsg))
	}

	// Get uniform locations
	projUniform = gl.GetUniformLocation(program, gl.Str("projection\x00"))
	if projUniform < 0 {
		log.Printf("Warning: projection uniform not found")
	}

	// Create VAO and VBO
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &vbo)

	// Create FBO and RBO for offscreen rendering
	gl.GenFramebuffers(1, &fbo)
	gl.GenRenderbuffers(1, &rbo)

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)
}

func CleanupOpenGL() {
	gl.DeleteProgram(program)
	gl.DeleteVertexArrays(1, &vao)
	gl.DeleteBuffers(1, &vbo)
	gl.DeleteFramebuffers(1, &fbo)
	gl.DeleteRenderbuffers(1, &rbo)
	glfw.Terminate()
}

func checkGLError(prefix string) error {
	err := gl.GetError()
	if err != 0 {
		return fmt.Errorf("%s: OpenGL error: %d", prefix, err)
	}
	return nil
}

func drawOffscreen(vertices []float32, size int32) []byte {
	// Set up framebuffer
	gl.BindFramebuffer(gl.FRAMEBUFFER, fbo)
	gl.BindRenderbuffer(gl.RENDERBUFFER, rbo)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.RGBA8, size, size)
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, rbo)

	if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		log.Printf("Framebuffer is not complete: %d", status)
	}
	if err := checkGLError("Framebuffer setup"); err != nil {
		log.Printf("OpenGL error: %v", err)
	}

	// Clear and set viewport
	gl.Viewport(0, 0, size, size)
	gl.ClearColor(1.0, 1.0, 1.0, 1.0)
	gl.Clear(gl.COLOR_BUFFER_BIT)

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

	// Draw lines
	gl.DrawArrays(gl.LINES, 0, int32(len(vertices)/2))
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
