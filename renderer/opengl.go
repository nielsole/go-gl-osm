package renderer

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log"
	"net/http"
	"strconv"

	"git.sr.ht/~sbinet/gg"
	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
	"github.com/nielsole/go-gl-osm/utils"
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
	dc := gg.NewContext(S, S)

	dc.SetRGB(1, 1, 1)
	dc.Clear()
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
	for _, wayReference := range *wayIndices {
		err := ReadMapObject(mmapData, int64(wayReference), &way)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// way := data.MapObjects[wayReference]
		visible := false
		if !bbox.overlaps(way.BoundingBox) {
			continue
		}
		//tags := way.Tags.Map()
		for i, point := range way.Points {
			// Print Lat and Lng of node
			//fmt.Printf("Lat: %f, Lon: %f\n", node.Lat, node.Lon)

			if bbox.contains(point) {
				visible = true
			}

			// We know that all previous points were outside of bounds, so we can skip them
			if i != 0 && visible {
				dc.SetRGB(0, 0, 0)
				dc.SetLineWidth(1)
				previousPoint := way.Points[i-1]
				currentPixel := pointToPixels(point, bbox, S)
				previousPixel := pointToPixels(previousPoint, bbox, S)
				dc.DrawLine(previousPixel.X, previousPixel.Y, currentPixel.X, currentPixel.Y)
				dc.Stroke()
			}
		}
	}

	w.Header().Set("Content-Type", "image/png")
	// var buf bytes.Buffer
	// png.Encode(&buf, img) // Encode the image to PNG
	// if err := dc.EncodePNG(w); err != nil {
	// 	http.Error(w, err.Error(), http.StatusInternalServerError)
	// }

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
