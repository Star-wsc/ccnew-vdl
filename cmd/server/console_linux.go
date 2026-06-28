//go:build !windows

package main

func hideConsoleWindow() {}

func showConsoleWindow() {}

func isConsoleVisible() bool {
	return false
}