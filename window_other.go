//go:build !windows && !(darwin && cgo)

package main

import "log"

// Sin motor de ventana (p. ej. binario de Mac compilado desde Windows):
// se usa el navegador del sistema.
func runWindow(url string) bool { return false }

func showError(msg string) { log.Println(msg) }
