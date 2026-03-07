//go:build wails && windows
// +build wails,windows

package main

import _ "unsafe"

// Wails defaults to resource icon ID=3 on Windows. Our resource pipeline
// (`rsrc -manifest ... -ico ...`) emits the application icon at ID=2.
// Override the internal Wails icon resource ID so taskbar/window icon uses
// our packaged icon instead of the default placeholder.
//
//go:linkname wailsAppIconID github.com/wailsapp/wails/v2/internal/frontend/desktop/windows/winc.AppIconID
var wailsAppIconID int

func init() {
	wailsAppIconID = 2
}
