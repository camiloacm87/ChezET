// mkrsrc genera rsrc_windows_amd64.syso con el icono y el manifiesto de la
// app de Windows, sin herramientas externas. El enlazador de Go incluye
// automáticamente los .syso del paquete main al compilar para Windows.
//
// Uso (desde la raíz del proyecto): go run ./tools/mkrsrc
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"sort"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtManifest  = 24
	langEnUS    = 0x0409
)

var iconSizes = []int{256, 128, 64, 48, 32, 24, 16}

const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <assemblyIdentity type="win32" name="AjedrezEnRed" version="1.1.0.0" processorArchitecture="*"/>
  <description>Ajedrez en Red</description>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0"
        processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
  <application xmlns="urn:schemas-microsoft-com:asm.v3">
    <windowsSettings>
      <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>
    </windowsSettings>
  </application>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security><requestedPrivileges><requestedExecutionLevel level="asInvoker" uiAccess="false"/></requestedPrivileges></security>
  </trustInfo>
</assembly>
`

type res struct {
	typ, id, lang uint32
	data          []byte
}

func le(v ...any) []byte {
	var b bytes.Buffer
	for _, x := range v {
		_ = binary.Write(&b, binary.LittleEndian, x)
	}
	return b.Bytes()
}

func main() {
	var list []res
	// Iconos (PNG dentro del recurso: soportado desde Windows Vista)
	var group bytes.Buffer
	group.Write(le(uint16(0), uint16(1), uint16(len(iconSizes))))
	for i, n := range iconSizes {
		png, err := os.ReadFile("packaging/icon/icon_" + itoa(n) + ".png")
		if err != nil {
			log.Fatal(err)
		}
		id := uint32(i + 1)
		list = append(list, res{rtIcon, id, langEnUS, png})
		dim := uint8(n)
		if n >= 256 {
			dim = 0
		}
		group.Write(le(dim, dim, uint8(0), uint8(0), uint16(1), uint16(32), uint32(len(png)), uint16(id)))
	}
	list = append(list, res{rtGroupIcon, 1, langEnUS, group.Bytes()})
	list = append(list, res{rtManifest, 1, langEnUS, []byte(manifest)})

	data, relocs := buildRsrc(list)
	writeCOFF("rsrc_windows_amd64.syso", data, relocs)
	fmt.Printf("rsrc_windows_amd64.syso generado (%d bytes de recursos)\n", len(data))
}

func itoa(n int) string { return string(bytesItoa(n)) }
func bytesItoa(n int) []byte {
	if n < 10 {
		return []byte{byte('0' + n)}
	}
	return append(bytesItoa(n/10), byte('0'+n%10))
}

// buildRsrc construye la sección .rsrc (árbol tipo → id → idioma) y devuelve
// los offsets de los campos que necesitan relocalización (RVA de los datos).
func buildRsrc(list []res) ([]byte, []uint32) {
	// Agrupar por tipo y por id (todo numérico, ordenado ascendente)
	types := map[uint32]map[uint32][]res{}
	for _, r := range list {
		if types[r.typ] == nil {
			types[r.typ] = map[uint32][]res{}
		}
		types[r.typ][r.id] = append(types[r.typ][r.id], r)
	}
	typeIDs := sortedKeys(types)

	dirSize := func(n int) int { return 16 + 8*n }
	// 1) calcular tamaños para colocar directorios
	off := dirSize(len(typeIDs))
	typeDirOff := map[uint32]int{}
	for _, t := range typeIDs {
		typeDirOff[t] = off
		off += dirSize(len(types[t]))
	}
	nameDirOff := map[[2]uint32]int{}
	for _, t := range typeIDs {
		for _, id := range sortedKeys(types[t]) {
			nameDirOff[[2]uint32{t, id}] = off
			off += dirSize(len(types[t][id]))
		}
	}
	// 2) entradas de datos (16 bytes cada una)
	type leaf struct {
		r     res
		entry int
		blob  int
	}
	var leaves []*leaf
	for _, t := range typeIDs {
		for _, id := range sortedKeys(types[t]) {
			for _, r := range types[t][id] {
				leaves = append(leaves, &leaf{r: r, entry: off})
				off += 16
			}
		}
	}
	// 3) datos alineados a 8 bytes
	for _, l := range leaves {
		off = (off + 7) &^ 7
		l.blob = off
		off += len(l.r.data)
	}
	buf := make([]byte, (off+7)&^7)
	put32 := func(at int, v uint32) { binary.LittleEndian.PutUint32(buf[at:], v) }
	dirHeader := func(at, n int) { binary.LittleEndian.PutUint16(buf[at+14:], uint16(n)) }

	// Directorio raíz
	dirHeader(0, len(typeIDs))
	for i, t := range typeIDs {
		put32(16+8*i, t)
		put32(16+8*i+4, 0x80000000|uint32(typeDirOff[t]))
	}
	li := 0
	var relocs []uint32
	for _, t := range typeIDs {
		ids := sortedKeys(types[t])
		dirHeader(typeDirOff[t], len(ids))
		for i, id := range ids {
			put32(typeDirOff[t]+16+8*i, id)
			put32(typeDirOff[t]+16+8*i+4, 0x80000000|uint32(nameDirOff[[2]uint32{t, id}]))
			nd := nameDirOff[[2]uint32{t, id}]
			rs := types[t][id]
			dirHeader(nd, len(rs))
			for j, r := range rs {
				l := leaves[li]
				li++
				put32(nd+16+8*j, r.lang)
				put32(nd+16+8*j+4, uint32(l.entry))
				put32(l.entry, uint32(l.blob)) // se relocaliza a RVA
				put32(l.entry+4, uint32(len(l.r.data)))
				relocs = append(relocs, uint32(l.entry))
				copy(buf[l.blob:], l.r.data)
			}
		}
	}
	return buf, relocs
}

func sortedKeys[V any](m map[uint32]V) []uint32 {
	k := make([]uint32, 0, len(m))
	for x := range m {
		k = append(k, x)
	}
	sort.Slice(k, func(i, j int) bool { return k[i] < k[j] })
	return k
}

// writeCOFF escribe un objeto COFF AMD64 con una única sección .rsrc.
func writeCOFF(path string, data []byte, relocs []uint32) {
	const hdr, sect = 20, 40
	rawPtr := uint32(hdr + sect)
	relPtr := rawPtr + uint32(len(data))
	symPtr := relPtr + uint32(10*len(relocs))
	var b bytes.Buffer
	// Cabecera de archivo
	b.Write(le(uint16(0x8664), uint16(1), uint32(0), symPtr, uint32(1), uint16(0), uint16(0)))
	// Cabecera de sección
	b.Write([]byte(".rsrc\x00\x00\x00"))
	b.Write(le(uint32(0), uint32(0), uint32(len(data)), rawPtr, relPtr, uint32(0),
		uint16(len(relocs)), uint16(0), uint32(0x40000040)))
	b.Write(data)
	for _, r := range relocs {
		b.Write(le(r, uint32(0), uint16(3))) // IMAGE_REL_AMD64_ADDR32NB → símbolo 0
	}
	// Tabla de símbolos: el símbolo de la sección .rsrc
	b.Write([]byte(".rsrc\x00\x00\x00"))
	b.Write(le(uint32(0), int16(1), uint16(0), uint8(3), uint8(0)))
	// Tabla de cadenas vacía
	b.Write(le(uint32(4)))
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}
