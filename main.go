// Ajedrez en Red — app de escritorio.
//
// Arranca el núcleo (paquete server) y muestra la interfaz en una ventana propia
// (WebView2 en Windows, WebKit en macOS, WebKitGTK en Linux). Si no hay motor de
// ventana disponible, abre el navegador del sistema como respaldo.
//
// Al cerrar la app se avisa al rival y se cierra la red (server.Shutdown).
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"chezzred/server"
)

const appTitle = "Ajedrez en Red"

func main() {
	port := flag.Int("port", 8765, "puerto de red que se abre al crear una partida")
	useBrowser := flag.Bool("browser", false, "abrir en el navegador en lugar de en ventana propia")
	headless := flag.Bool("headless", false, "solo el servicio, sin ventana ni navegador (para pruebas)")
	flag.Parse()
	log.SetFlags(log.Ltime)

	url, err := server.Start(*port)
	if err != nil {
		showError("No se pudo iniciar el juego:\n" + err.Error())
		return
	}

	// Cerrar con Ctrl+C o cerrando la consola también avisa al rival.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		server.Shutdown()
		os.Exit(0)
	}()

	if *headless {
		fmt.Println(url)
		select {}
	}

	if !*useBrowser && runWindow(url) {
		server.Shutdown() // se cerró la ventana
		return
	}

	// Modo navegador (respaldo)
	fmt.Println("==============================================")
	fmt.Println("   ♞  AJEDREZ EN RED")
	fmt.Println("==============================================")
	fmt.Printf("  Abriendo: %s\n", url)
	fmt.Println("  La red solo se abre al crear una partida.")
	fmt.Println("  Deja esta ventana abierta mientras juegas;")
	fmt.Println("  ciérrala para salir (se avisará a tu rival).")
	fmt.Println("==============================================")
	time.Sleep(200 * time.Millisecond)
	openBrowser(url)
	select {}
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Printf("Abre manualmente en tu navegador: %s", url)
	}
}
