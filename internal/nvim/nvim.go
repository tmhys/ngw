// Package nvim starts or connects to a Neovim server and wraps the API
// calls ngw uses. The bootstrap Lua and the paste logic follow ng
// (https://github.com/Shougo/ng, MIT).
//
// ngw starts "nvim --embed" and talks msgpack-rpc over its stdin/stdout.
// --embed makes Neovim wait until a UI has attached before it reads the
// user's config, exactly like the builtin TUI, so &lines/&columns and
// nvim_list_uis() behave as in a terminal while the config runs.
package nvim

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"ngw/internal/rpc"
)

// Nvim is a connection to a Neovim server.
type Nvim struct {
	*rpc.Client
	Proc *os.Process // nil when attached to an existing server
	Chan int64       // this client's channel id
}

type pipeConn struct {
	io.ReadCloser
	w io.WriteCloser
}

func (p pipeConn) Write(b []byte) (int, error) { return p.w.Write(b) }
func (p pipeConn) Close() error {
	p.w.Close()
	return p.ReadCloser.Close()
}

// Find returns the nvim executable to run: path if given, else nvim(.exe)
// from PATH, else the standard install locations.
func Find(path string) (string, error) {
	if path != "" {
		return path, nil
	}
	if p, err := exec.LookPath("nvim"); err == nil {
		return p, nil
	}
	if runtime.GOOS == "windows" {
		var cands []string
		for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
			if d := os.Getenv(env); d != "" {
				cands = append(cands, filepath.Join(d, "Neovim", "bin", "nvim.exe"))
			}
		}
		if d := os.Getenv("LOCALAPPDATA"); d != "" {
			cands = append(cands,
				filepath.Join(d, "Programs", "Neovim", "bin", "nvim.exe"),
				filepath.Join(d, "Microsoft", "WinGet", "Links", "nvim.exe"))
		}
		if d := os.Getenv("USERPROFILE"); d != "" {
			cands = append(cands, filepath.Join(d, "scoop", "shims", "nvim.exe"))
		}
		if d := os.Getenv("ChocolateyInstall"); d != "" {
			cands = append(cands, filepath.Join(d, "bin", "nvim.exe"))
		}
		for _, c := range cands {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	}
	return "", fmt.Errorf("nvim not found: install Neovim (winget install Neovim.Neovim) or pass --nvim PATH")
}

// Start launches nvimPath --embed with args and connects to it.
func Start(nvimPath string, args []string, onNotify rpc.NotificationHandler, onRequest rpc.RequestHandler) (*Nvim, error) {
	// g:ngw is set with --cmd, before the user's config runs, so that
	// "if exists('g:ngw')" works in init.vim / init.lua.
	cmdArgs := append([]string{"--embed", "--cmd", "let g:ngw = 1"}, args...)
	cmd := exec.Command(nvimPath, cmdArgs...)
	hideConsole(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", nvimPath, err)
	}
	go cmd.Wait()
	n := &Nvim{Client: rpc.NewClient(pipeConn{stdout, stdin}, onNotify, onRequest), Proc: cmd.Process}
	return n, nil
}

// Connect attaches to an existing server listening on host:port
// (nvim --listen 127.0.0.1:6666).
func Connect(addr string, onNotify rpc.NotificationHandler, onRequest rpc.RequestHandler) (*Nvim, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Nvim{Client: rpc.NewClient(conn, onNotify, onRequest)}, nil
}

// Prepare runs the bootstrap Lua. Serve must already be running.
func (n *Nvim) Prepare() error {
	info, err := n.Call("nvim_get_api_info")
	if err != nil {
		return err
	}
	if l, ok := info.([]any); ok && len(l) > 0 {
		switch v := l[0].(type) {
		case int64:
			n.Chan = v
		case uint64:
			n.Chan = int64(v)
		}
	}
	_, err = n.Call("nvim_exec_lua", Bootstrap, []any{n.Chan})
	return err
}

// Attach registers ngw as a UI. Only ext_linegrid is used: the command
// line, messages, popup menu and tabline are drawn by Neovim itself,
// exactly as in a terminal.
func (n *Nvim) Attach(cols, rows int) error {
	_, err := n.Call("nvim_ui_attach", cols, rows, map[string]any{
		"rgb":          true,
		"ext_linegrid": true,
	})
	return err
}

// TryResize asks Neovim to resize the grid. It does not wait.
func (n *Nvim) TryResize(cols, rows int) error {
	return n.Notify("nvim_ui_try_resize", cols, rows)
}

// Input sends keys in Neovim's key notation. It does not wait.
func (n *Nvim) Input(keys string) error {
	if keys == "" {
		return nil
	}
	return n.Notify("nvim_input", keys)
}

// InputMouse sends a mouse event. It does not wait.
func (n *Nvim) InputMouse(button, action, modifier string, row, col int) error {
	return n.Notify("nvim_input_mouse", button, action, modifier, 0, row, col)
}

// SetFocus tells Neovim whether the window has focus (FocusGained/Lost).
func (n *Nvim) SetFocus(focused bool) error {
	return n.Notify("nvim_ui_set_focus", focused)
}

// Command runs an Ex command without waiting.
func (n *Nvim) Command(cmd string) error { return n.Notify("nvim_command", cmd) }

// ExecLuaAsync runs Lua without waiting for the result.
func (n *Nvim) ExecLuaAsync(code string, args ...any) error {
	if args == nil {
		args = []any{}
	}
	return n.Notify("nvim_exec_lua", code, args)
}

var pasteSeq atomic.Int64

// Paste inserts text as a terminal's bracketed paste does (vim.paste()).
// The text is stored in a variable first and the paste itself goes through
// the input queue as a <Cmd> key, so it neither overtakes keys typed
// before it nor falls behind keys typed after it. It waits for Neovim to
// store the text; call it from a goroutine.
func (n *Nvim) Paste(text string) error {
	if text == "" {
		return nil
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	name := fmt.Sprintf("ngwpaste_%d", pasteSeq.Add(1))
	if _, err := n.Call("nvim_set_var", name, text); err != nil {
		return err
	}
	return n.Input("<Cmd>lua require'ngw_paste'('" + name + "')<CR>")
}

// Notifications sent by the bootstrap Lua.
const (
	// UIEnterNotification: the UI has entered (the user's config has run).
	UIEnterNotification = "ngw_ui_enter"
	// SetSizeNotification: the user set 'lines' or 'columns' (cols, lines).
	SetSizeNotification = "ngw_set_size"
	// VarNotification: a g:ngw_* variable changed (name, value).
	VarNotification = "ngw_var"
	// Requests for the clipboard provider.
	ClipGetRequest = "ngw_clip_get"
	ClipSetRequest = "ngw_clip_set"
)

const pasteLua = `
package.loaded.ngw_paste = function(name)
  local text = vim.g[name]
  vim.g[name] = nil
  if not text then return end
  local lines = vim.split(text, '\n', { plain = true })
  if vim.fn.getcmdtype() ~= '' then
    local line, pos = vim.fn.getcmdline(), vim.fn.getcmdpos()
    vim.fn.setcmdline(line:sub(1, pos - 1) .. lines[1] .. line:sub(pos), pos + #lines[1])
    return
  end
  vim.paste(lines, -1)
end`

const uiEnterLua = `
vim.api.nvim_create_autocmd('UIEnter', {
  once = true,
  callback = function() vim.rpcnotify(chan, '` + UIEnterNotification + `') end,
})`

const setSizeLua = `
vim.api.nvim_create_autocmd('OptionSet', {
  pattern = { 'lines', 'columns' },
  callback = function()
    vim.rpcnotify(chan, '` + SetSizeNotification + `', vim.o.columns, vim.o.lines)
  end,
})`

const varsLua = `
vim.cmd(('call dictwatcheradd(g:, "ngw_*", {d, k, z -> rpcnotify(%d, "` + VarNotification + `", k, get(z, "new", v:null))})'):format(chan))
for k, v in pairs(vim.fn.eval([[filter(copy(g:), 'v:key =~# "^ngw_"')]])) do
  vim.rpcnotify(chan, '` + VarNotification + `', k, v)
end`

// clipboardLua installs the Windows clipboard (CF_UNICODETEXT) as the "+"
// and "*" registers, unless the user's config sets g:clipboard itself.
// It runs from VimEnter so that the config gets the first word (or at
// once when attaching to a server that has started); no
// win32yank.exe is needed.
const clipboardLua = `
local function install_clipboard()
    if vim.g.clipboard ~= nil or vim.g.ngw_clipboard == 0 then return end
    local function copy(lines, regtype) vim.rpcrequest(chan, '` + ClipSetRequest + `', lines, regtype) end
    local function paste() return vim.rpcrequest(chan, '` + ClipGetRequest + `') end
    vim.g.clipboard = {
      name = 'ngw',
      copy = { ['+'] = copy, ['*'] = copy },
      paste = { ['+'] = paste, ['*'] = paste },
      cache_enabled = 0,
    }
    if vim.fn.exists('g:loaded_clipboard_provider') == 1 then
      vim.g.loaded_clipboard_provider = nil
      pcall(vim.cmd, 'runtime autoload/provider/clipboard.vim')
    end
end
if vim.v.vim_did_enter == 1 then
  install_clipboard()
else
  vim.api.nvim_create_autocmd('VimEnter', { once = true, callback = install_clipboard })
end`

// dropLua opens dropped files, like GVim.
const DropLua = `
local files = {}
for _, f in ipairs(...) do files[#files + 1] = vim.fn.fnameescape(f) end
if #files > 0 then vim.cmd('drop ' .. table.concat(files, ' ')) end`

// Bootstrap is the Lua run in Neovim before attaching; its argument is
// ngw's channel id.
const Bootstrap = "local chan = ...\nvim.g.ngw = 1\n" + pasteLua + "\n" + uiEnterLua + "\n" + setSizeLua + "\n" + varsLua + "\n" + clipboardLua
