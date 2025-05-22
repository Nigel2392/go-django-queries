//go:build debug
// +build debug

package queries

import "fmt"

const DEBUGGING = true

func DebugPrintf(format string, args ...any) {
	fmt.Printf(format, args...)
}

func DebugPrintln(args ...any) {
	fmt.Println(args...)
}
