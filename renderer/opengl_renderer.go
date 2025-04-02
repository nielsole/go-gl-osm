package renderer

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"log"
	"sync"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

type OpenGLRenderer struct {
	verticesBuffer []float32
	window         *glfw.Window // Store window reference
	renderLock     sync.Mutex   // Lock for OpenGL operations
}

func (r *OpenGLRenderer) InitOpenGL() error {
	r.renderLock.Lock()
	defer r.renderLock.Unlock()

	// Enable blending for proper alpha handling
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// Initialize shaders and program
	vertexShader := gl.CreateShader(gl.VERTEX_SHADER)
	vertexShaderCString, free := gl.Strs(vertexShaderSource)
	gl.ShaderSource(vertexShader, 1, vertexShaderCString, nil)
	free()
	gl.CompileShader(vertexShader)
	if err := checkShaderError(vertexShader); err != nil {
		return fmt.Errorf("vertex shader error: %v", err)
	}

	fragmentShader := gl.CreateShader(gl.FRAGMENT_SHADER)
	fragmentShaderCString, free := gl.Strs(fragmentShaderSource)
	gl.ShaderSource(fragmentShader, 1, fragmentShaderCString, nil)
	free()
	gl.CompileShader(fragmentShader)
	if err := checkShaderError(fragmentShader); err != nil {
		return fmt.Errorf("fragment shader error: %v", err)
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
		return fmt.Errorf("program link error: %s", string(logMsg))
	}

	// Get uniform locations
	projUniform = gl.GetUniformLocation(program, gl.Str("projection\x00"))
	if projUniform < 0 {
		log.Printf("Warning: projection uniform not found")
	}

	// Create VAO and VBO
	gl.GenVertexArrays(1, &vao)
	gl.GenBuffers(1, &vbo)

	// Create FBO and texture for offscreen rendering
	gl.GenFramebuffers(1, &fbo)
	gl.GenTextures(1, &texture)

	// Initialize texture
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, 256, 256, 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.BindTexture(gl.TEXTURE_2D, 0)

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)
	return nil
}

func (r *OpenGLRenderer) cleanupOpenGL() {
	gl.DeleteProgram(program)
	gl.DeleteVertexArrays(1, &vao)
	gl.DeleteBuffers(1, &vbo)
	gl.DeleteFramebuffers(1, &fbo)
	gl.DeleteTextures(1, &texture)
}

func NewOpenGLRenderer() (*OpenGLRenderer, error) {

	if err := glfw.Init(); err != nil {
		return nil, fmt.Errorf("failed to initialize glfw: %v", err)
	}
	// no need to lock here, since the renderer has not been created yet

	glfw.WindowHint(glfw.Visible, glfw.False)
	glfw.WindowHint(glfw.ContextVersionMajor, 4)
	glfw.WindowHint(glfw.ContextVersionMinor, 1)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Decorated, glfw.False)
	glfw.WindowHint(glfw.Focused, glfw.False)
	glfw.WindowHint(glfw.AutoIconify, glfw.False)
	glfw.WindowHint(glfw.Resizable, glfw.False)

	window, err := glfw.CreateWindow(256, 256, "", nil, nil)
	if err != nil {
		glfw.Terminate()
		return nil, fmt.Errorf("failed to create window: %v", err)
	}
	window.MakeContextCurrent()

	if err := gl.Init(); err != nil {
		window.Destroy()
		glfw.Terminate()
		return nil, fmt.Errorf("failed to initialize go-gl: %v", err)
	}

	renderer := &OpenGLRenderer{
		verticesBuffer: make([]float32, 0, 4194304),
		window:         window,
		renderLock:     sync.Mutex{},
	}

	if err := renderer.InitOpenGL(); err != nil {
		renderer.Close()
		return nil, fmt.Errorf("failed to initialize OpenGL resources: %v", err)
	}

	setupFramebuffer(256)

	return renderer, nil
}

func (r *OpenGLRenderer) Close() {
	r.renderLock.Lock()
	defer r.renderLock.Unlock()

	if r.window != nil {
		r.cleanupOpenGL()
		r.window.Destroy()
		r.window = nil
	}
	glfw.Terminate()
}

func (r *OpenGLRenderer) RenderTile(data *Data, mmapData *[]byte, x, y, z uint32) image.Image {
	// Use existing drawOffscreen function
	vertices := r.prepareTileVertices(data, mmapData, x, y, z)
	imgBytes := drawOffscreen(vertices, 256)
	img, _ := png.Decode(bytes.NewReader(imgBytes))
	return img
}

func (r *OpenGLRenderer) prepareTileVertices(data *Data, mmapData *[]byte, x, y, z uint32) []float32 {
	tile := Tile{X: x, Y: y, Z: z}
	bbox := getBoundingBox(tile)
	const S = 256

	// Reset the buffer length while keeping capacity
	r.verticesBuffer = r.verticesBuffer[:0]

	wayIndices, ok := data.Tiles[tile.index()]
	if !ok {
		return r.verticesBuffer
	}

	way := MapObject{Points: make([]Point, 0, data.MaxPoints)}
	for _, wayReference := range *wayIndices {
		ReadMapObject(mmapData, int64(wayReference), &way)
		if !bbox.overlaps(way.BoundingBox) {
			continue
		}

		// Pre-grow the slice if needed
		requiredCap := len(r.verticesBuffer) + (len(way.Points)-1)*4 // 4 float32s per line segment
		if requiredCap > cap(r.verticesBuffer) {
			newCap := cap(r.verticesBuffer) * 2
			fmt.Println("newCap", newCap, "requiredCap", requiredCap)
			if newCap < requiredCap {
				newCap = requiredCap
			}
			newBuffer := make([]float32, len(r.verticesBuffer), newCap)
			copy(newBuffer, r.verticesBuffer)
			r.verticesBuffer = newBuffer
		}

		for i := 0; i < len(way.Points)-1; i++ {
			p1 := pointToPixels(way.Points[i], bbox, S)
			p2 := pointToPixels(way.Points[i+1], bbox, S)
			r.verticesBuffer = append(r.verticesBuffer,
				float32(p1.X), float32(p1.Y),
				float32(p2.X), float32(p2.Y))
		}
	}
	return r.verticesBuffer
}
