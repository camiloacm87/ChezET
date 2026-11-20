//go:build !windows && !((darwin || linux) && cgo)

package main

import "log"

// Sin motor de ventana (binario compilado sin cgo): se usa el navegador del sistema.
func runWindow(url string) bool { return false }

func showError(msg string) { log.Println(msg) }
