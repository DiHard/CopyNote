package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jchv/go-webview2"
)

// Single-file frontend produced by Vite + vite-plugin-singlefile.
// Served over loopback HTTP — NavigateToString gives an about:blank
// origin that breaks ES modules, so we use http://127.0.0.1:PORT/
// instead. Only one request is made (HTML+CSS+JS are all inlined).
//
//go:embed all:web/dist
var distFS embed.FS

// Chromium switches passed to WebView2 to disable background services
// that add startup latency on first navigation (Safe Browsing updater,
// domain reliability telemetry, component updater, etc.).
var browserArgs = []string{
	"--disable-background-networking",
	"--disable-component-update",
	"--disable-domain-reliability",
	"--disable-sync",
	"--disable-breakpad",
	"--disable-client-side-phishing-detection",
	"--disable-default-apps",
	"--disable-features=AutofillServerCommunication,CertificateTransparencyComponentUpdater,OptimizationHints",
	"--no-pings",
	"--no-first-run",
	"--no-default-browser-check",
	"--no-service-autorun",
	"--metrics-recording-only",
}

func main() {
	// Persistent WebView2 user-data folder in %LOCALAPPDATA%\CopyNote\WebView2
	// so cache/cookies/state survive between runs.
	var dataPath string
	if appData := os.Getenv("LOCALAPPDATA"); appData != "" {
		dataPath = filepath.Join(appData, "CopyNote", "WebView2")
		if err := os.MkdirAll(dataPath, 0o755); err != nil {
			log.Printf("mkdir datapath: %v", err)
		}
	}

	// WebView2 picks up this env var before spawning msedgewebview2.exe.
	os.Setenv("WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS", strings.Join(browserArgs, " "))

	// Loopback HTTP server serving the embedded frontend.
	staticFS, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		log.Fatalf("embed sub: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	go func() {
		mux := http.NewServeMux()
		mux.Handle("/", http.FileServer(http.FS(staticFS)))
		_ = http.Serve(ln, mux)
	}()
	url := fmt.Sprintf("http://%s/", ln.Addr().String())

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		DataPath:  dataPath,
		WindowOptions: webview2.WindowOptions{
			Title:  "CopyNote",
			Width:  420,
			Height: 640,
			Center: true,
		},
	})
	if w == nil {
		log.Fatal("failed to create webview")
	}
	defer w.Destroy()

	// Bridge: simple ping to validate Go ↔ JS bridge works.
	if err := w.Bind("ping", func(msg string) string {
		return "pong: " + msg
	}); err != nil {
		log.Fatalf("bind ping: %v", err)
	}

	w.Navigate(url)
	w.Run()
}
