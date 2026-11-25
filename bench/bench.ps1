# Compares Neovim GUIs on Windows: startup time, memory, and the CPU time
# the GUI process spends drawing a scroll workload (2000 lines at 125/s,
# like a held key) and a full-redraw workload (100 colorscheme changes).
# Usage: .\bench\bench.ps1 -Out results   (GUIs listed in $guis below)
param([string]$Out = "results", [string]$File = "bench\sample.txt")
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing

$root = (Resolve-Path ".").Path
$init = Join-Path $root "bench\init.lua"
$file = (Resolve-Path $File).Path
New-Item -ItemType Directory -Force $Out | Out-Null
$Out = (Resolve-Path $Out).Path

$guis = @(
  @{ name = "ngw";      exe = $env:NGW_EXE;    proc = "ngw";       args = @("--geometry", "120x40", "--", "-u", $init, $file) },
  @{ name = "Nvy";      exe = $env:NVY_EXE;    proc = "Nvy";       args = @("--geometry=120x40", "-u", $init, $file) },
  @{ name = "Neovide";  exe = $env:NEOVIDE_EXE; proc = "neovide";  args = @("--no-fork", "--", "-u", $init, $file) },
  @{ name = "nvim-qt";  exe = $env:NVIMQT_EXE; proc = "nvim-qt";   args = @("--", "-u", $init, $file) }
)

function NowMs { [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds() }
function WaitFile($path, $sec) {
  $deadline = (Get-Date).AddSeconds($sec)
  while (-not (Test-Path $path)) {
    if ((Get-Date) -gt $deadline) { throw "timeout waiting for $(Split-Path $path -Leaf)" }
    Start-Sleep -Milliseconds 20
  }
  Start-Sleep -Milliseconds 20
  Get-Content $path -Raw | ConvertFrom-Json
}
function GuiProc($g) {
  Get-Process -Name $g.proc -ErrorAction SilentlyContinue | Sort-Object StartTime -Descending | Select-Object -First 1
}
function KillAll {
  foreach ($n in @("ngw", "Nvy", "neovide", "nvim-qt", "nvim")) {
    Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
  }
  Start-Sleep -Milliseconds 500
}
function Shot($path) {
  $b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
  $bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height
  $g = [System.Drawing.Graphics]::FromImage($bmp)
  $g.CopyFromScreen($b.Location, [System.Drawing.Point]::Empty, $b.Size)
  $bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
  $g.Dispose(); $bmp.Dispose()
}
function Launch($g, $dir, $mode) {
  Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Force $dir | Out-Null
  $env:BENCH_OUT = $dir
  $env:BENCH_MODE = $mode
  $t = NowMs
  Start-Process -FilePath $g.exe -ArgumentList $g.args | Out-Null
  $ready = WaitFile (Join-Path $dir "ready.json") 60
  return [double]$ready.t - $t
}

$results = @()
foreach ($g in $guis) {
  $r = [ordered]@{ gui = $g.name; startup_ms = $null; working_set_mb = $null; private_mb = $null;
    scroll_cpu_ms = $null; redraw_cpu_ms = $null; grid = $null; error = $null }
  try {
    if (-not $g.exe -or -not (Test-Path $g.exe)) { throw "not installed" }
    KillAll
    # Startup: one warm-up, then the median of 5.
    $times = @()
    for ($i = 0; $i -lt 6; $i++) {
      $ms = Launch $g (Join-Path $Out "$($g.name)-startup") "startup"
      if ($i -gt 0) { $times += $ms }
      Start-Sleep -Milliseconds 1500
      KillAll
    }
    $r.startup_ms = [math]::Round(($times | Sort-Object)[2])

    # Workloads.
    $dir = Join-Path $Out "$($g.name)-run"
    Launch $g $dir "full" | Out-Null
    Start-Sleep -Seconds 3   # let startup work settle
    $p = GuiProc $g
    if (-not $p) { throw "GUI process not found" }
    $p.Refresh()
    $r.working_set_mb = [math]::Round($p.WorkingSet64 / 1MB, 1)
    $r.private_mb = [math]::Round($p.PrivateMemorySize64 / 1MB, 1)
    $c0 = $p.TotalProcessorTime.TotalMilliseconds
    New-Item -ItemType File (Join-Path $dir "go_scroll") | Out-Null
    $s = WaitFile (Join-Path $dir "scroll_done.json") 120
    Start-Sleep -Seconds 1   # let the GUI finish drawing
    $p.Refresh(); $c1 = $p.TotalProcessorTime.TotalMilliseconds
    $r.scroll_cpu_ms = [math]::Round($c1 - $c0)
    $r.grid = "$($s.columns)x$($s.lines)"
    Shot (Join-Path $Out "$($g.name)-scroll.png")
    New-Item -ItemType File (Join-Path $dir "go_redraw") | Out-Null
    WaitFile (Join-Path $dir "redraw_done.json") 120 | Out-Null
    Start-Sleep -Seconds 1
    $p.Refresh(); $c2 = $p.TotalProcessorTime.TotalMilliseconds
    $r.redraw_cpu_ms = [math]::Round($c2 - $c1)
    Shot (Join-Path $Out "$($g.name)-redraw.png")
    New-Item -ItemType File (Join-Path $dir "go_quit") | Out-Null
  } catch {
    $r.error = $_.Exception.Message
    try { Shot (Join-Path $Out "$($g.name)-error.png") } catch {}
  }
  KillAll
  $results += [pscustomobject]$r
  Write-Host ($r | ConvertTo-Json -Compress)
}

$results | ConvertTo-Json | Set-Content (Join-Path $Out "results.json") -Encoding utf8
$md = @("| GUI | 起動 (ms, 中央値) | メモリ WS (MB) | Private (MB) | スクロール中のCPU (ms) | 全再描画中のCPU (ms) | グリッド | エラー |",
        "|---|---:|---:|---:|---:|---:|---|---|")
foreach ($r in $results) {
  $md += "| $($r.gui) | $($r.startup_ms) | $($r.working_set_mb) | $($r.private_mb) | $($r.scroll_cpu_ms) | $($r.redraw_cpu_ms) | $($r.grid) | $($r.error) |"
}
$md -join "`n" | Set-Content (Join-Path $Out "results.md") -Encoding utf8
if ($env:GITHUB_STEP_SUMMARY) { $md -join "`n" | Add-Content $env:GITHUB_STEP_SUMMARY -Encoding utf8 }
$md -join "`n"
