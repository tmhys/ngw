-- Benchmark config shared by every GUI. The driver (bench.ps1) and Neovim
-- talk through files in $BENCH_OUT, so the GUI under test needs no
-- special support.
local out = assert(os.getenv('BENCH_OUT'), 'BENCH_OUT not set')
local mode = os.getenv('BENCH_MODE') or 'full'

local function now()
  local s, us = vim.uv.gettimeofday()
  return s * 1000 + us / 1000
end
local function write(name, t)
  local f = assert(io.open(out .. '/' .. name, 'w'))
  f:write(vim.json.encode(t))
  f:close()
end
local function exists(name) return vim.uv.fs_stat(out .. '/' .. name) ~= nil end

vim.o.guifont = 'Consolas:h12'
vim.o.number = true
vim.o.cursorline = true
vim.o.swapfile = false
vim.o.shada = ''
vim.cmd('syntax on')
vim.cmd.colorscheme('habamax')

-- Run step(i) n times, one every interval ms (like a held key), then done().
local function run(n, interval, step, done)
  local i, t = 0, vim.uv.new_timer()
  t:start(interval, interval, vim.schedule_wrap(function()
    i = i + 1
    step(i)
    if i >= n then
      t:stop()
      t:close()
      done()
    end
  end))
end

-- Wait until the driver creates the file `name`.
local function await(name, fn)
  local t = vim.uv.new_timer()
  t:start(50, 50, vim.schedule_wrap(function()
    if exists(name) then
      t:stop()
      t:close()
      fn()
    end
  end))
end

vim.api.nvim_create_autocmd('UIEnter', {
  once = true,
  callback = function()
    write('ready.json', { t = now() })
    if mode == 'startup' then
      vim.defer_fn(function() vim.cmd('qall!') end, 200)
      return
    end
    -- The same grid for every GUI (fits the runner's 1024x768 screen).
    vim.o.columns = 100
    vim.o.lines = 30
    await('go_scroll', function()
      local t0 = now()
      run(2000, 8, function() vim.cmd('normal! \5') end, function()
        write('scroll_done.json', { t = now(), ms = now() - t0, lines = vim.o.lines, columns = vim.o.columns })
        await('go_redraw', function()
          local schemes = { 'habamax', 'default' }
          local t1 = now()
          run(100, 40, function(i) vim.cmd.colorscheme(schemes[i % 2 + 1]) end, function()
            write('redraw_done.json', { t = now(), ms = now() - t1 })
            await('go_quit', function() vim.cmd('qall!') end)
          end)
        end)
      end)
    end)
  end,
})
