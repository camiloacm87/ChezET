//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
)

func init() { runtime.LockOSThread() }

// runWindow muestra la interfaz en una ventana propia con WebView2.
// Devuelve false si WebView2 no está disponible.
func runWindow(url string) bool {
	dataDir := ""
	if d, err := os.UserConfigDir(); err == nil {
		dataDir = filepath.Join(d, "AjedrezEnRed")
	}
	w := webview2.NewWithOptions(webview2.WebViewOptions{
		AutoFocus: true,
		DataPath:  dataDir,
		WindowOptions: webview2.WindowOptions{
			Title:  appTitle,
			Width:  1180,
			Height: 860,
			IconId: 1,
			Center: true,
		},
	})
	if w == nil {
		showError("No se encontró Microsoft Edge WebView2 en este equipo.\n" +
			"El juego se abrirá en el navegador. Para usar la ventana propia, instala " +
			"\"WebView2 Runtime\" desde la web de Microsoft.")
		return false
	}
	defer w.Destroy()
	w.SetSize(1180, 860, webview2.HintNone)
	w.Navigate(url)
	w.Run()
	return true
}

func showError(msg string) {
	user32 := syscall.NewLazyDLL("user32.dll")
	box := user32.NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(appTitle)
	m, _ := syscall.UTF16PtrFromString(msg)
	_, _, _ = box.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x30)
}
