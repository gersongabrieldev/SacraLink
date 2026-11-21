//go:build !windows

package main

// attachParentConsole: fora do Windows o servidor já é um programa de console.
func attachParentConsole() {}
