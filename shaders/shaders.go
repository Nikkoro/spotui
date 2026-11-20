// Package shaders carries the Windows Terminal pixel shader spotui feeds
// (see internal/ui/wtshader.go), so a release binary can write it out.
package shaders

import (
	"bytes"
	_ "embed"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Source is shaders/spotui.hlsl.
//
//go:embed spotui.hlsl
var Source []byte

// FileName is the name the shader is written under.
const FileName = "spotui.hlsl"

// Install writes the shader to path unless it is already there as is, and
// reports whether it wrote.
func Install(path string) (bool, error) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, Source) {
		return false, nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, Source, 0o644)
}

// Refresh brings a shader written earlier up to date with this build, so
// it keeps reading the cells the way this spotui writes them. A missing
// file is left missing.
func Refresh(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		return false, nil
	}
	return Install(path)
}
