# Local Go dependencies

The root `go.mod` redirects module resolution to these directories. These
checked-in sources, rather than the module cache, are used for every build.
They were originally copied to work around a local Go TLS certificate issue.

| Directory | Upstream module | Recorded revision / provenance |
| --- | --- | --- |
| `go-webview2` | `github.com/jchv/go-webview2` | Root module records `56598839c808`; local window/background changes described below |
| `go-winloader` | `github.com/jchv/go-winloader` | Root module records `c1995be93bd1` |
| `sys` | `golang.org/x/sys` | Root module records `beda7e5e158e`, but this copy declares Go 1.25; its exact upstream revision has not been verified |
| `rsrc` | `github.com/akavel/rsrc` | Resource compiler source; exact copied revision has not been verified |

The revisions above are the existing manifest declarations, not an assertion
that the directories are pristine upstream checkouts. Preserve each dependency's
license files. No vendored dependency was upgraded during the reliability fixes.

## Local WebView2 changes

Repository history contains adaptations for the frameless window, transparent
background, and silent startup. In particular, `436cb5d` changes startup window
visibility/style; `d453300` contains earlier window/settings integration.
Inspect `git log -p -- third_party/go-webview2` before replacing this directory.
CopyNote's asynchronous update bridge now lives in `internal/bridge`; it does
not change the vendored library's synchronous RPC execution semantics.

## Updating a dependency

1. Record the exact upstream commit and compare it with the checked-in source.
2. Preserve or deliberately replace the local changes listed above.
3. Update the root `go.mod`, this document, and the dependency's license files.
4. Run the Windows CI checks and manually verify tray startup, window show/hide,
   clipboard, themes, and shutdown. Updating source without checking these native
   scenarios is insufficient.
