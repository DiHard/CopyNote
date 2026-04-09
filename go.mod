module copynote

go 1.25.0

require (
	github.com/jchv/go-webview2 v0.0.0-00010101000000-000000000000
	golang.org/x/sys v0.0.0-20210218145245-beda7e5e158e
)

require github.com/jchv/go-winloader v0.0.0-20250406163304-c1995be93bd1 // indirect

replace (
	github.com/jchv/go-webview2 => ./third_party/go-webview2
	github.com/jchv/go-winloader => ./third_party/go-winloader
	golang.org/x/sys => ./third_party/sys
)
