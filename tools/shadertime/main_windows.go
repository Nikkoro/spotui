//go:build windows

// Command shadertime compiles the Windows Terminal shader with Windows'
// own d3dcompiler_47.dll, the compiler Windows Terminal uses when a tab
// opens, and reports whether it compiles, how long it took, the bytecode
// size and the compiler's messages. A slow compile is a slow first start
// of every tab with the shader.
//
//	go run ./tools/shadertime                  (on Windows)
//	GOOS=windows go build -o /tmp/st.exe ./tools/shadertime && /tmp/st.exe shaders/spotui.hlsl   (from WSL)
//
// -each also compiles it with each effect in effect() left out, to see
// what each costs; -max fails when the compile takes longer.
package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var d3dCompile = syscall.NewLazyDLL("d3dcompiler_47.dll").NewProc("D3DCompile")

// Flags for D3DCompile, as Windows Terminal passes them.
const (
	packMatrixColumnMajor = 1 << 4
	enableStrictness      = 1 << 11
	optimizationLevel3    = 1 << 15
)

// blob is an ID3DBlob: IUnknown's three methods, then GetBufferPointer
// and GetBufferSize.
type blob struct{ vtbl *[5]uintptr }

func (b *blob) bytes() []byte {
	if b == nil {
		return nil
	}
	p, _, _ := syscall.SyscallN(b.vtbl[3], uintptr(unsafe.Pointer(b)))
	n, _, _ := syscall.SyscallN(b.vtbl[4], uintptr(unsafe.Pointer(b)))
	// p is memory the blob owns, not Go's: read it through a pointer to
	// p rather than converting the uintptr straight back.
	return slices.Clone(unsafe.Slice(*(**byte)(unsafe.Pointer(&p)), n))
}

func (b *blob) release() {
	if b != nil {
		syscall.SyscallN(b.vtbl[2], uintptr(unsafe.Pointer(b)))
	}
}

type result struct {
	took time.Duration
	size int
	msgs string
	err  error
}

func compile(src []byte, target string) result {
	name, _ := syscall.BytePtrFromString("spotui.hlsl")
	entry, _ := syscall.BytePtrFromString("main")
	tgt, _ := syscall.BytePtrFromString(target)
	var code, msgs *blob
	start := time.Now()
	hr, _, _ := d3dCompile.Call(uintptr(unsafe.Pointer(&src[0])), uintptr(len(src)),
		uintptr(unsafe.Pointer(name)), 0, 0, uintptr(unsafe.Pointer(entry)), uintptr(unsafe.Pointer(tgt)),
		packMatrixColumnMajor|enableStrictness|optimizationLevel3, 0,
		uintptr(unsafe.Pointer(&code)), uintptr(unsafe.Pointer(&msgs)))
	r := result{took: time.Since(start), size: len(code.bytes()), msgs: strings.TrimSpace(string(msgs.bytes()))}
	code.release()
	msgs.release()
	if int32(hr) < 0 {
		r.err = fmt.Errorf("does not compile (0x%08x)", uint32(hr))
	}
	return r
}

// best compiles src runs times and keeps the fastest, the least disturbed
// by whatever else the machine was doing.
func best(src []byte, target string, runs int) result {
	r := compile(src, target)
	for i := 1; i < runs && r.err == nil; i++ {
		if n := compile(src, target); n.took < r.took {
			r.took = n.took
		}
	}
	return r
}

func main() {
	target := flag.String("target", "ps_4_0", "shader model, as Windows Terminal compiles it")
	runs := flag.Int("runs", 3, "compile this many times and report the fastest")
	each := flag.Bool("each", false, "also compile without each effect, to see what each costs")
	maxTook := flag.Duration("max", 0, "fail when the compile takes longer than this (0: never)")
	flag.Parse()
	path := "shaders/spotui.hlsl"
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	r := best(src, *target, *runs)
	if r.msgs != "" {
		fmt.Println(r.msgs)
	}
	if r.err != nil {
		fmt.Fprintf(os.Stderr, "%s %v\n", path, r.err)
		os.Exit(1)
	}
	fmt.Printf("%s: %v, %d bytes (%s)\n", path, r.took.Round(time.Millisecond), r.size, *target)

	if *each {
		// Each effect's call in effect(), replaced by black in turn.
		for _, call := range regexp.MustCompile(`c = ([A-Za-z]+)\(tex, m\);`).FindAllSubmatch(src, -1) {
			without := strings.Replace(string(src), string(call[0]), "c = float3(0.0, 0.0, 0.0);", 1)
			w := best([]byte(without), *target, *runs)
			fmt.Printf("  %-10s %6v  (without it %v)\n", call[1], (r.took - w.took).Round(time.Millisecond), w.took.Round(time.Millisecond))
		}
	}
	if *maxTook > 0 && r.took > *maxTook {
		fmt.Fprintf(os.Stderr, "%s takes %v to compile, over %v: every tab with the shader opens that much slower\n",
			path, r.took.Round(time.Millisecond), *maxTook)
		os.Exit(1)
	}
}
