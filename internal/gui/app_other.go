//go:build !windows

package gui

import (
	"errors"

	"ngw/internal/nvim"
)

// Options are the command line choices that concern the window.
type Options struct {
	Cols, Rows int
	Maximized  bool
	Attached   bool
	Connect    func(onNotify func(string, []any), onRequest func(string, []any) (any, error)) (*nvim.Nvim, error)
}

// Run is only implemented on Windows.
func Run(opts Options) error { return errors.New("ngw runs on Windows only") }
