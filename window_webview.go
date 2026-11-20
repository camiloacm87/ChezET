//go:build (darwin || linux) && cgo

package main

import (
	"log"
	"runtime"

	webview "github.com/webview/webview_go"
)

// macOS y GTK exigen que la interfaz corra en el hilo principal.
func init() { runtime.LockOSThread() }

// runWindow muestra la interfaz en una ventana propia
// (WebKit en macOS, WebKitGTK en Linux).
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
