//go:build darwin && cgo

package main

import (
	"log"
	"runtime"

	webview "github.com/webview/webview_go"
)

func init() { runtime.LockOSThread() } // macOS exige la UI en el hilo principal

// runWindow muestra la interfaz en una ventana propia (WebKit).
func runWindow(url string) bool {
	w := webview.New(false)
	if w == nil {
		return false
	}
	defer w.Destroy()
	w.SetTitle(appTitle)
	w.SetSize(1180, 860, webview.HintNone)
	w.Navigate(url)
	w.Run()
	return true
}

func showError(msg string) { log.Println(msg) }
