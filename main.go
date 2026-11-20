// Ajedrez en Red — app de escritorio.
//
// Arranca el núcleo (paquete server) y muestra la interfaz en una ventana propia
// (WebView2 en Windows, WebKit en macOS). Si no hay motor de ventana disponible,
// abre el navegador del sistema como respaldo.
package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"chezzred/server"
)

const appTitle = "Ajedrez en Red"

func main() {
	port := flag.Int("port", 8765, "puerto TCP para la partida")
	useBrowser := flag.Bool("browser", false, "abrir en el navegador en lugar de en ventana propia")
	flag.Parse()
	log.SetFlags(log.Ltime)

	url, err := server.Start(*port)
	if err != nil {
		showError("No se pudo iniciar el juego:\n" + err.Error())
		return
	}

	if !*useBrowser && runWindow(url) {
		return // se cerró la ventana: salir
	}

	// Modo navegador (respaldo)
	primary, ips := server.Addresses()
	fmt.Println("==============================================")
	fmt.Println("   ♞  AJEDREZ EN RED")
	fmt.Println("==============================================")
	fmt.Printf("  Abriendo: %s\n", url)
	if primary != "" {
		fmt.Printf("  Tu dirección en la red: %s\n", primary)
	}
	if len(ips) > 1 {
		fmt.Printf("  Otras IPs de este equipo: %s\n", strings.Join(ips[1:], ", "))
	}
	fmt.Println("  Deja esta ventana abierta mientras juegas.")
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
