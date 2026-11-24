package main

import "ngw/internal/win32"

// report shows a message: ngw is a GUI program and has no console.
func report(msg string) { win32.MessageBox(0, msg, "ngw", 0x40) }
