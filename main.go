package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/pprof"
	"os"
	"runtime"

	"github.com/nielsole/go-gl-osm/renderer"
)

func init() {
	// GLFW event handling must run on the main thread
	runtime.LockOSThread()
}

func main() {
	http_listen_host := flag.String("host", "0.0.0.0", "HTTP Listening host")
	http_listen_port := flag.Int("port", 8080, "HTTP Listening port")
	https_listen_port := flag.Int("tls_port", 8443, "HTTPS Listening port. This listener is only enabled if both tls cert and key are set.")
	static_dir := flag.String("static", "./static/", "Path to static file directory")
	osm_path := flag.String("osm_path", "", "Path to osm_path to use for direct rendering. (experimental)")
	tls_cert_path := flag.String("tls_cert_path", "", "Path to TLS certificate")
	tls_key_path := flag.String("tls_key_path", "", "Path to TLS key")
	verbose := flag.Bool("verbose", false, "Output debug log messages")

	flag.Parse()

	var requestHandler func(http.ResponseWriter, *http.Request)

	if len(*osm_path) == 0 {
		fmt.Println("No OSM file provided via `osm_path`. Exiting.")
		os.Exit(1)
	}
	// Create a temp file.
	var err error
	tempFile, err := ioutil.TempFile("", "example")
	if err != nil {
		fmt.Println("Cannot create temp file:", err)
		os.Exit(1)
	}

	data, err := renderer.LoadData(*osm_path, 15, tempFile)
	if err != nil {
		logFatalf("There was an error loading data: %v", err)
	}
	tempFileName := tempFile.Name()
	tempFile.Close()

	// Memory-map the file
	mmapData, mmapFile, err := renderer.Mmap(tempFileName)
	if err != nil {
		logFatalf("There was an error memory-mapping temp file: %v", err)
	}
	defer renderer.Munmap(mmapData)
	defer mmapFile.Close()
	// HTTP request multiplexer
	httpServeMux := http.NewServeMux()

	requestHandler = func(w http.ResponseWriter, r *http.Request) {
		if *verbose {
			logDebugf("%s request received: %s", r.Method, r.RequestURI)
		}
		if r.Method != "GET" {
			http.Error(w, "Only GET requests allowed", http.StatusMethodNotAllowed)
			return
		}
		renderer.HandleRenderRequestOpenGL(w, r, data, 15, mmapData)
	}
	defer func() {
		// Cleanup the temp file.
		if err := os.Remove(tempFile.Name()); err != nil {
			fmt.Println("Failed to remove temp file:", err)
		} else {
			fmt.Println("Temp file removed.")
		}
	}()

	// Tile HTTP request handler
	httpServeMux.HandleFunc("/tile/", requestHandler)

	// Static HTTP request handler
	httpServeMux.Handle("/", http.FileServer(http.Dir(*static_dir)))
	httpServeMux.HandleFunc("/debug/pprof/", http.HandlerFunc(pprof.Index))
	httpServeMux.HandleFunc("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	httpServeMux.HandleFunc("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	httpServeMux.HandleFunc("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	httpServeMux.HandleFunc("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))

	// Register other pprof handlers
	httpServeMux.Handle("/debug/pprof/goroutine", pprof.Handler("goroutine"))
	httpServeMux.Handle("/debug/pprof/heap", pprof.Handler("heap"))
	httpServeMux.Handle("/debug/pprof/threadcreate", pprof.Handler("threadcreate"))
	httpServeMux.Handle("/debug/pprof/block", pprof.Handler("block"))

	// Start HTTP server in a goroutine
	go func() {
		httpServer := &http.Server{
			Addr:    fmt.Sprintf("%s:%d", *http_listen_host, *http_listen_port),
			Handler: httpServeMux,
		}

		if len(*tls_cert_path) > 0 && len(*tls_key_path) > 0 {
			go func() {
				logInfof("Starting HTTPS server on %s:%d", *http_listen_host, *https_listen_port)
				httpsServer := &http.Server{
					Addr:    fmt.Sprintf("%s:%d", *http_listen_host, *https_listen_port),
					Handler: httpServeMux,
				}
				if err := httpsServer.ListenAndServeTLS(*tls_cert_path, *tls_key_path); err != http.ErrServerClosed {
					logFatalf("Failed to start HTTPS server: %v", err)
				}
			}()
		} else {
			logInfof("TLS is disabled")
		}

		logInfof("Starting HTTP server on %s:%d", *http_listen_host, *http_listen_port)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			logFatalf("Failed to start HTTP server: %v", err)
		}
	}()

	// Initialize OpenGL and run render loop on main thread
	renderer.InitOpenGL()
	defer renderer.CleanupOpenGL()
	renderer.RenderLoop(context.Background())
}
