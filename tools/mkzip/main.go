// mkzip crea un .zip conservando permisos Unix (necesario para macOS).
// Uso: go run ./tools/mkzip salida.zip CarpetaDentroDelZip archivo1 archivo2 ...
// Los archivos .txt/.md se guardan como 0644; el resto como ejecutables 0755.
package main

import (
	"archive/zip"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 4 {
		log.Fatal("uso: mkzip salida.zip carpeta archivos...")
	}
	out, err := os.Create(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	dir := os.Args[2]
	for _, f := range os.Args[3:] {
		src, err := os.Open(f)
		if err != nil {
			log.Fatal(err)
		}
		name := filepath.Base(f)
		mode := os.FileMode(0o755)
		if ext := strings.ToLower(filepath.Ext(name)); ext == ".txt" || ext == ".md" {
			mode = 0o644
		}
		h := &zip.FileHeader{Name: dir + "/" + name, Method: zip.Deflate, Modified: time.Now()}
		h.SetMode(mode)
		h.CreatorVersion = 3<<8 | 20 // creado en "Unix" para que macOS respete los permisos
		w, err := zw.CreateHeader(h)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := io.Copy(w, src); err != nil {
			log.Fatal(err)
		}
		src.Close()
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}
}
