// Package android es el punto de entrada de gomobile bind para la app Android.
// Reexporta el paquete server de ChezET sin ninguna dependencia de escritorio.
package android

import "chezzred/server"

// Reexportar los símbolos que usa la Activity de Kotlin.

// Start arranca el servidor y devuelve la URL local.
func Start(lanPort int) (string, error) { return server.Start(lanPort) }

// Shutdown cierra el servidor limpiamente.
func Shutdown() { server.Shutdown() }

// SetLocalNetwork proporciona la IP Wi-Fi desde el lado Android.
func SetLocalNetwork(primary, ipsCSV string) { server.SetLocalNetwork(primary, ipsCSV) }
