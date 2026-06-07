//go:build darwin

package main

/*
#cgo darwin LDFLAGS: -framework Cocoa
void installWenhaoSystemQuitHook(void);
*/
import "C"

import "sync"

var installSystemQuitHookOnce sync.Once

func installSystemQuitHook() {
	installSystemQuitHookOnce.Do(func() {
		C.installWenhaoSystemQuitHook()
	})
}

//export WenhaoMarkSystemQuit
func WenhaoMarkSystemQuit() {
	markSystemQuitRequested()
}
