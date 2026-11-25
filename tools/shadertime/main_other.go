//go:build !windows

// Command shadertime times the Windows Terminal shader's compile with
// Windows' own compiler, so it runs on Windows only; from WSL, build it
// for Windows and run the .exe (see main_windows.go).
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "shadertime needs Windows' d3dcompiler_47.dll. From WSL:")
	fmt.Fprintln(os.Stderr, "  GOOS=windows go build -o /tmp/shadertime.exe ./tools/shadertime && /tmp/shadertime.exe shaders/spotui.hlsl")
	os.Exit(2)
}
