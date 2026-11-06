//go:build darwin

package macui

/*
#include <stdlib.h>
#include "macui.h"
*/
import "C"

// Development aids for the devtools build of the app; nothing in a normal
// build calls them. Main thread only.

var (
	evalNext    int
	evalWaiting = map[int]func(string){}
)

// EvalResult evaluates js and passes its result, as text, to done.
func EvalResult(js string, done func(string)) {
	evalNext++
	evalWaiting[evalNext] = done
	withCString(js, func(p *C.char) { C.macuiEvalResult(C.int(evalNext), p) })
}

// Snapshot saves what the page shows as a PNG at path; done gets "ok" or an
// error.
func Snapshot(path string, done func(string)) {
	evalNext++
	evalWaiting[evalNext] = done
	withCString(path, func(p *C.char) { C.macuiSnapshot(C.int(evalNext), p) })
}

//export goEvalResult
func goEvalResult(id C.int, text *C.char) {
	done := evalWaiting[int(id)]
	delete(evalWaiting, int(id))
	if done != nil {
		done(C.GoString(text))
	}
}
