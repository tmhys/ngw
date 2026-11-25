# Compares Neovim GUIs on Windows: startup time, memory, and the CPU time
# the GUI process spends drawing a scroll workload (2000 lines at 125/s,
# like a held key) and a full-redraw workload (100 colorscheme changes).
# CPU time is split into user (the program itself) and kernel (mostly
# GDI/win32k and window management) time. Workloads run -Repeat times;
# medians are reported.
#
#   .\bench\bench.ps1 -Out results [-Only ngw,Nvy] [-Repeat 3] [-Startups 5]
param([string]$Out = "results", [string]$File = "bench\sample.txt",
      [string]$Only = "", [int]$Repeat = 3, [int]$Startups = 5)
$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Windows.Forms, System.Drawing
# CPU cycles actually used. GetProcessTimes (TotalProcessorTime) charges
# whole 15.6 ms clock ticks to whichever thread is running when the tick
# fires, which badly misjudges programs that wake often for a short time
# (exactly what a GUI following a held key does). Cycle counts do not
# sample: they add up what each thread really ran.
Add-Type @"
using System; using System.Runtime.InteropServices;
public static class Cyc {
  [DllImport("kernel32.dll")] public static extern bool QueryProcessCycleTime(IntPtr h, out ulong c);
  [DllImport("kernel32.dll")] public static extern IntPtr OpenThread(int access, bool inherit, int id);
  [DllImport("kernel32.dll")] public static extern bool QueryThreadCycleTime(IntPtr h, out ulong c);
  [DllImport("kernel32.dll")] public static extern bool CloseHandle(IntPtr h);
}
"@
$mhz = [double](Get-CimInstance Win32_Processor | Select-Object -First 1).MaxClockSpeed
Write-Host "CPU: $((Get-CimInstance Win32_Processor | Select-Object -First 1).Name), $mhz MHz, $([Environment]::ProcessorCount) logical"

$root = (Resolve-Path ".").Path
$init = Join-Path $root "bench\init.lua"
$file = (Resolve-Path $File).Path
New-Item -ItemType Directory -Force $Out | Out-Null
$Out = (Resolve-Path $Out).Path

$guis = @(
  @{ name = "ngw";     exe = $env:NGW_EXE;     proc = "ngw";     args = @("--geometry", "100x30", "--", "-u", $init, $file) },
  # A/B variants: the current build with more Ps, and an older build
  # (NGW_REF_EXE, from the workflow's "ref" input) with one P and default.
  @{ name = "ngw-mp";     exe = $env:NGW_EXE;     proc = "ngw"; args = @("--geometry", "100x30", "--", "-u", $init, $file); env = @{ GOMAXPROCS = "2" } },
  @{ name = "ngw-ref-p1"; exe = $env:NGW_REF_EXE; proc = "ngw"; args = @("--geometry", "100x30", "--", "-u", $init, $file); env = @{ GOMAXPROCS = "1" } },
  @{ name = "ngw-ref-mp"; exe = $env:NGW_REF_EXE; proc = "ngw"; args = @("--geometry", "100x30", "--", "-u", $init, $file); env = @{ GOMAXPROCS = "2" } },
  @{ name = "Nvy";     exe = $env:NVY_EXE;     proc = "Nvy";     args = @("--geometry=100x30", "-u", $init, $file) },
  @{ name = "Neovide"; exe = $env:NEOVIDE_EXE; proc = "neovide"; args = @("--no-fork", "--", "-u", $init, $file) },
  @{ name = "nvim-qt"; exe = $env:NVIMQT_EXE;  proc = "nvim-qt"; args = @("--", "-u", $init, $file) }
)
if ($Only) { $names = $Only -split ','; $guis = @($guis | Where-Object { $names -contains $_.name }) }

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
  $gr = [System.Drawing.Graphics]::FromImage($bmp)
  $gr.CopyFromScreen($b.Location, [System.Drawing.Point]::Empty, $b.Size)
  $bmp.Save($path, [System.Drawing.Imaging.ImageFormat]::Png)
  $gr.Dispose(); $bmp.Dispose()
}
function Launch($g, $dir, $mode) {
  Remove-Item -Recurse -Force $dir -ErrorAction SilentlyContinue
  New-Item -ItemType Directory -Force $dir | Out-Null
  $env:BENCH_OUT = $dir
  $env:BENCH_MODE = $mode
  if ($g.env) { foreach ($k in @($g.env.Keys)) { Set-Item "Env:$k" $g.env[$k] } }
  $t = NowMs
  Start-Process -FilePath $g.exe -ArgumentList $g.args | Out-Null
  if ($g.env) { foreach ($k in @($g.env.Keys)) { Remove-Item "Env:$k" -ErrorAction SilentlyContinue } }
  $ready = WaitFile (Join-Path $dir "ready.json") 60
  return [double]$ready.t - $t
}
function Cpu($p) {
  $p.Refresh()
  $c = [uint64]0
  [Cyc]::QueryProcessCycleTime($p.Handle, [ref]$c) | Out-Null
  @{ user = $p.UserProcessorTime.TotalMilliseconds; kernel = $p.PrivilegedProcessorTime.TotalMilliseconds; cyc = [double]$c }
}
function Threads($p) {
  $p.Refresh()
  $h = @{}
  foreach ($t in $p.Threads) {
    $c = [uint64]0
    $th = [Cyc]::OpenThread(0x0800, $false, $t.Id)   # THREAD_QUERY_LIMITED_INFORMATION
    if ($th -ne [IntPtr]::Zero) { [Cyc]::QueryThreadCycleTime($th, [ref]$c) | Out-Null; [Cyc]::CloseHandle($th) | Out-Null }
    $h[$t.Id] = @{ c = [double]$c }
  }
  return $h
}
function Median($xs) {
  $s = @($xs | Sort-Object)
  if ($s.Count -eq 0) { return $null }
  return [math]::Round($s[[int][math]::Floor(($s.Count - 1) / 2)])
}

# One run of both workloads; returns the GUI's CPU time for each.
function Workloads($g, $dir, $shots) {
  Launch $g $dir "full" | Out-Null
  Start-Sleep -Seconds 3   # let startup and the resize settle
  $p = GuiProc $g
  if (-not $p) { throw "GUI process not found" }
  $p.Refresh()
  $m = @{ ws = $p.WorkingSet64 / 1MB; priv = $p.PrivateMemorySize64 / 1MB }
  $c0 = Cpu $p
  $th0 = Threads $p
  New-Item -ItemType File (Join-Path $dir "go_scroll") | Out-Null
  $s = WaitFile (Join-Path $dir "scroll_done.json") 180
  Start-Sleep -Seconds 1   # let the GUI finish drawing
  $c1 = Cpu $p
  $th1 = Threads $p
  $tl = foreach ($id in $th1.Keys) {
    $b = if ($th0.ContainsKey($id)) { $th0[$id].c } else { 0 }
    [pscustomobject]@{ id = $id; ms = ($th1[$id].c - $b) / ($mhz * 1000) }
  }
  Write-Host "threads $($g.name): $(@($th1.Keys).Count) total; busiest (ms from cycles): $(($tl | Sort-Object ms -Descending | Select-Object -First 5 | ForEach-Object { "$($_.id)=$([math]::Round($_.ms))" }) -join ' ')"
  if ($shots) { Shot (Join-Path $Out "$($g.name)-scroll.png") }
  New-Item -ItemType File (Join-Path $dir "go_redraw") | Out-Null
  WaitFile (Join-Path $dir "redraw_done.json") 180 | Out-Null
  Start-Sleep -Seconds 1
  $c2 = Cpu $p
  if ($shots) { Shot (Join-Path $Out "$($g.name)-redraw.png") }
  New-Item -ItemType File (Join-Path $dir "go_quit") | Out-Null
  # Let the GUI exit by itself (ngw writes its profile and stats then).
  $p.WaitForExit(10000) | Out-Null
  return @{
    mem = $m; grid = "$($s.columns)x$($s.lines)"; kcells = [double]$s.columns * [double]$s.lines / 1000
    scroll_user = $c1.user - $c0.user; scroll_kernel = $c1.kernel - $c0.kernel
    redraw_user = $c2.user - $c1.user; redraw_kernel = $c2.kernel - $c1.kernel
    scroll_cyc_ms = ($c1.cyc - $c0.cyc) / ($mhz * 1000); redraw_cyc_ms = ($c2.cyc - $c1.cyc) / ($mhz * 1000)
  }
}

# Startup first, then the workloads in rounds: every round runs each GUI
# once, so a VM that gets faster or slower over time affects all GUIs
# alike.
$state = [ordered]@{}
foreach ($g in $guis) {
  $state[$g.name] = @{ g = $g; runs = @(); startup = $null; error = $null }
  try {
    if (-not $g.exe -or -not (Test-Path $g.exe)) { throw "not installed" }
    KillAll
    if ($Startups -gt 0) {
      $times = @()
      for ($i = 0; $i -le $Startups; $i++) {   # run 0 is a warm-up
        $ms = Launch $g (Join-Path $Out "$($g.name)-startup") "startup"
        if ($i -gt 0) { $times += $ms }
        Start-Sleep -Milliseconds 1500
        KillAll
      }
      $state[$g.name].startup = Median $times
    }
  } catch { $state[$g.name].error = $_.Exception.Message }
}
for ($i = 0; $i -lt $Repeat; $i++) {
  foreach ($g in $guis) {
    $s = $state[$g.name]
    if ($s.error) { continue }
    try {
      if ($i -eq $Repeat - 1 -and $env:NGW_PROFILE_DIR -and $g.name -eq "ngw") {
        $env:NGW_STATS = Join-Path $env:NGW_PROFILE_DIR "stats.json"
      }
      $s.runs += Workloads $g (Join-Path $Out "$($g.name)-run$i") ($i -eq 0)
    } catch {
      $s.error = $_.Exception.Message
      try { Shot (Join-Path $Out "$($g.name)-error.png") } catch {}
    }
    Remove-Item Env:NGW_CPUPROFILE, Env:NGW_STATS -ErrorAction SilentlyContinue
    KillAll
  }
}

$results = @()
foreach ($name in $state.Keys) {
  $s = $state[$name]; $runs = $s.runs
  $r = [ordered]@{ gui = $name; startup_ms = $s.startup; working_set_mb = $null; private_mb = $null
    scroll_cpu_ms = $null; scroll_user_ms = $null; scroll_kernel_ms = $null; scroll_cpu_per_kcell = $null
    redraw_cpu_ms = $null; redraw_user_ms = $null; redraw_kernel_ms = $null; redraw_cpu_per_kcell = $null
    scroll_cyc_ms = $null; redraw_cyc_ms = $null; scroll_cyc_runs = $null
    scroll_runs = $null; grid = $null; error = $s.error }
  if ($runs.Count -gt 0) {
    $r.working_set_mb = [math]::Round(($runs | ForEach-Object { $_.mem.ws } | Sort-Object)[0], 1)
    $r.private_mb = [math]::Round(($runs | ForEach-Object { $_.mem.priv } | Sort-Object)[0], 1)
    $r.grid = $runs[0].grid
    $k = $runs[0].kcells
    $r.scroll_user_ms = Median ($runs | ForEach-Object { $_.scroll_user })
    $r.scroll_kernel_ms = Median ($runs | ForEach-Object { $_.scroll_kernel })
    $r.scroll_cpu_ms = Median ($runs | ForEach-Object { $_.scroll_user + $_.scroll_kernel })
    $r.redraw_user_ms = Median ($runs | ForEach-Object { $_.redraw_user })
    $r.redraw_kernel_ms = Median ($runs | ForEach-Object { $_.redraw_kernel })
    $r.redraw_cpu_ms = Median ($runs | ForEach-Object { $_.redraw_user + $_.redraw_kernel })
    $r.scroll_cpu_per_kcell = [math]::Round($r.scroll_cpu_ms / $k)
    $r.redraw_cpu_per_kcell = [math]::Round($r.redraw_cpu_ms / $k)
    $r.scroll_runs = ($runs | ForEach-Object { [math]::Round($_.scroll_user + $_.scroll_kernel) }) -join "/"
    $r.scroll_cyc_ms = Median ($runs | ForEach-Object { $_.scroll_cyc_ms })
    $r.redraw_cyc_ms = Median ($runs | ForEach-Object { $_.redraw_cyc_ms })
    $r.scroll_cyc_runs = ($runs | ForEach-Object { [math]::Round($_.scroll_cyc_ms) }) -join "/"
  }
  $results += [pscustomobject]$r
  Write-Host ($r | ConvertTo-Json -Compress)
}

$results | ConvertTo-Json | Set-Content (Join-Path $Out "results.json") -Encoding utf8
$md = @("CPU時間はサイクル数から換算（$mhz MHz）。括弧内は従来の GetProcessTimes の値。",
        "",
        "| GUI | 起動 (ms) | メモリ WS (MB) | スクロールCPU (ms) | 各回 | 全再描画CPU (ms) | グリッド | エラー |",
        "|---|---:|---:|---:|---|---:|---|---|")
foreach ($r in $results) {
  $md += "| $($r.gui) | $($r.startup_ms) | $($r.working_set_mb) | $($r.scroll_cyc_ms) ($($r.scroll_cpu_ms)) | $($r.scroll_cyc_runs) | $($r.redraw_cyc_ms) ($($r.redraw_cpu_ms)) | $($r.grid) | $($r.error) |"
}
$md -join "`n" | Set-Content (Join-Path $Out "results.md") -Encoding utf8
if ($env:GITHUB_STEP_SUMMARY) { $md -join "`n" | Add-Content $env:GITHUB_STEP_SUMMARY -Encoding utf8 }
$md -join "`n"
