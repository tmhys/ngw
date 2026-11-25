// Command ngw is a fast, minimal Neovim GUI for Windows with full
// Japanese input method support. It follows the design of ng
// (https://github.com/Shougo/ng): Neovim draws everything, ngw is just
// the window.
//
//	ngw [--nvim PATH] [--geometry COLSxROWS] [--maximized] [--] [nvim arguments...]
//	ngw --server HOST:PORT
package main

import (
	"fmt"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"

	"ngw/internal/gui"
	"ngw/internal/nvim"
)

var version = "dev"

const usage = `usage: ngw [options] [--] [nvim arguments...]

options:
  --nvim PATH            nvim.exe to run (default: from PATH)
  --server HOST:PORT     attach to a running "nvim --listen HOST:PORT"
  --geometry COLSxROWS   initial grid size (default 100x32)
  --maximized            start maximized
  --version              print the version
  -h, --help             show this help

Everything else is passed to nvim, e.g. "ngw -O a.txt b.txt".`

type config struct {
	nvimPath, server string
	cols, rows       int
	maximized        bool
	args             []string
}

func parse(args []string) (*config, error) {
	c := &config{cols: 100, rows: 32}
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs an argument", a)
			}
			i++
			return args[i], nil
		}
		var err error
		switch {
		case a == "--":
			c.args = append(c.args, args[i+1:]...)
			return c, nil
		case a == "--nvim":
			c.nvimPath, err = next()
		case a == "--server":
			c.server, err = next()
		case a == "--geometry":
			var g string
			if g, err = next(); err == nil {
				err = c.geometry(g)
			}
		case strings.HasPrefix(a, "--geometry="):
			err = c.geometry(strings.TrimPrefix(a, "--geometry="))
		case a == "--maximized":
			c.maximized = true
		case a == "-h" || a == "--help":
			return nil, errHelp
		case a == "--version":
			return nil, errVersion
		default:
			c.args = append(c.args, a)
		}
		if err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *config) geometry(g string) error {
	w, h, ok := strings.Cut(strings.ToLower(g), "x")
	cols, err1 := strconv.Atoi(w)
	rows, err2 := strconv.Atoi(h)
	if !ok || err1 != nil || err2 != nil || cols < 1 || rows < 1 {
		return fmt.Errorf("bad --geometry %q (want COLSxROWS, e.g. 120x40)", g)
	}
	c.cols, c.rows = cols, rows
	return nil
}

type sentinel string

func (s sentinel) Error() string { return string(s) }

const (
	errHelp    = sentinel("help")
	errVersion = sentinel("version")
)

func main() {
	cfg, err := parse(os.Args[1:])
	switch err {
	case nil:
	case errHelp:
		report(usage)
		return
	case errVersion:
		report("ngw " + version)
		return
	default:
		report("ngw: " + err.Error() + "\n\n" + usage)
		os.Exit(2)
	}
	opts := gui.Options{
		Cols: cfg.cols, Rows: cfg.rows, Maximized: cfg.maximized, Attached: cfg.server != "",
		Connect: func(onNotify func(string, []any), onRequest func(string, []any) (any, error)) (*nvim.Nvim, error) {
			if cfg.server != "" {
				return nvim.Connect(cfg.server, onNotify, onRequest)
			}
			path, err := nvim.Find(cfg.nvimPath)
			if err != nil {
				return nil, err
			}
			return nvim.Start(path, cfg.args, onNotify, onRequest)
		},
	}
	// NGW_CPUPROFILE=file writes a Go CPU profile (for benchmarks).
	if path := os.Getenv("NGW_CPUPROFILE"); path != "" {
		if f, err := os.Create(path); err == nil {
			pprof.StartCPUProfile(f)
			defer func() { pprof.StopCPUProfile(); f.Close() }()
		}
	}
	if err := gui.Run(opts); err != nil {
		report("ngw: " + err.Error())
		os.Exit(1)
	}
}
