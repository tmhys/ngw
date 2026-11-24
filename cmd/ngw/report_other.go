//go:build !windows

package main

import "fmt"

func report(msg string) { fmt.Println(msg) }
