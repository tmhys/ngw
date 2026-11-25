# ngw

**Windows 向け・日本語完全対応の、軽くて速い Neovim GUI。**

[Shougo/ng](https://github.com/Shougo/ng)（Linux 専用の最小 Neovim GUI）の設計思想と
コア（msgpack-RPC・グリッド管理・文字幅）を受け継ぎ、描画と入力を Win32
（GDI + IMM32）で書き直したものです。cgo なしの純 Go、依存ライブラリなし、
単体 exe（約 2.6MB）で動きます。

> ngw は ng とは別のプロジェクトで、ng の作者による公認・保守はありません。
> 不具合の報告は ng ではなく、このリポジトリへお願いします。

![ngw で日本語を編集](docs/screenshots/edit.png)

## 特徴

- **日本語入力（IME）をインライン表示**：変換中の文字列はカーソル位置に直接描画され、
  変換候補ウィンドウは変換中の文節の真下に出ます（`IMR_QUERYCHARPOSITION` にも応答）。

  ![変換前](docs/screenshots/ime-preedit.png)
  ![変換後](docs/screenshots/ime-converted.png)
- **ノーマルモードで IME が邪魔しない**：挿入・置換・コマンドライン・ターミナルモード以外では
  IME を切り離すので、IME をオンにしたまま `Esc` しても `dd` や `:w` がそのまま効きます。
  挿入モードに戻ると IME の状態（オン/オフ）が復元されます。`:` のコマンド入力では IME はオフで始まります
  （`:w` が変換待ちにならないように）。いつ IME を使えるかは `g:ngw_ime` で細かく決めることもできます。
- **全角・半角の正しい描画**：Neovim が決めたセル幅に必ず収めます。全角文字はセル 2 つ分に、
  `ambiwidth=single` で幅広に描かれる記号（○ ※ → など）はセル内に横縮小して描くので、
  はみ出しや描画の残骸が出ません。
- **フォントのフォールバック**：`guifont` にない文字は `guifontwide` → BIZ UDゴシック →
  MS ゴシック → 游ゴシック → Nerd Font → Segoe UI Symbol … の順に自動で探します。
- **クリップボードがそのまま使える**：`"+y` `"+p` が Windows のクリップボード
  （UTF-16）と直結。win32yank.exe 不要で、行単位/文字単位の区別も保ちます。
- **速い**：
  - Neovim が `flush` した完全な画面だけを描く（描きかけを見せない・無駄な描画をしない）
  - 変化した行だけを再描画し、スクロールは画素を `BitBlt` で動かすだけ
    （スクロール後の画面と全再描画した画面がピクセル単位で一致することを確認済み）
  - 同じ色・同じフォントの連続文字は `ExtTextOutW` 1 回で描画
  - アニメーションなし、カーソル点滅なし
- **高 DPI 対応**（Per-Monitor v2）。モニタ間の移動でもぼやけません。
- ファイルのドラッグ＆ドロップ（`:drop`）、マウス（クリック・ドラッグ・ホイール・横ホイール）、
  `:set guifont=*` でフォント選択ダイアログ、ダークテーマ時は自動でタイトルバーも暗色。

ng と同じく、**画面は Neovim が描いたものだけ**を表示します（`ext_linegrid` のみ）。
GUI 独自のタブ・メニュー・サイドバーはありません。ターミナルで動くものはそのまま動きます。

## ベンチマーク

GitHub Actions の Windows Server 2025 VM（4 vCPU、GPU なし）3 台で、[Nvy](https://github.com/RMichelsen/Nvy) と
[nvim-qt](https://github.com/equalsraf/neovim-qt) と比べた結果です（2026-11-25、Neovim 0.12.6）。
値は GUI プロセスが使った CPU 時間（ms、CPU サイクル数から換算）で、**小さいほど軽い**です。
各 GUI を 1 周ずつ順番を入れ替えながら 5 周し、その中央値です。

**スクロール**：約 6,000 行のファイルを 1 行ずつ 2,000 回（キーを押しっぱなしにした速さ、毎秒 125 回）

| VM | **ngw** | Nvy | nvim-qt※ |
|---|---:|---:|---:|
| 1 | **973** | 2,369 | 1,833 |
| 2 | **879** | 2,908 | 1,774 |
| 3 | **621** | 1,579 | 1,305 |

**全面再描画**：カラースキームの切り替え 100 回

| VM | **ngw** | Nvy | nvim-qt※ |
|---|---:|---:|---:|
| 1 | **110** | 479 | 416 |
| 2 | **103** | 221 | 495 |
| 3 | **71** | 344 | 361 |

起動時間（起動から Neovim の UIEnter まで）とメモリ使用量（GUI プロセスのワーキングセット）も
ngw が最小でした（約 90〜120 ms、約 16 MB。Nvy は約 25 MB、nvim-qt は約 48 MB）。

注意：

- **GPU のない VM での結果**です。GPU で描画する Nvy（Direct2D）は、GPU のある PC では差が縮まる
  可能性があります。Neovide は GPU がないと CPU で描画することになり公平に比べられないため、除外しました。
- ※ nvim-qt は 100×30 の指定に従わず 56×36（約 3 分の 2 のセル数）で描いています。
- キーを押してから画面に出るまでの遅延（体感の速さ）は測っていません。
- 同じ exe を 2 つ並べて測ったときの差は ±1% 以内でした。
- 手順は [bench/bench.ps1](bench/bench.ps1)、[.github/workflows/bench.yml](.github/workflows/bench.yml)。
  Actions の bench ワークフローから誰でも再実行できます。

## インストール

### 1. Neovim を入れる（0.10 以降）

PowerShell で：

```powershell
winget install Neovim.Neovim
```

### 2. ngw.exe をダウンロードする

1. [Actions の ci ワークフロー](https://github.com/tmhys/ngw/actions/workflows/ci.yml) を開き、
   一番上の緑のチェックの付いた実行をクリック
2. ページ下部の **Artifacts** にある `ngw-windows-amd64` をクリックしてダウンロード
   （GitHub へのログインが必要です）
3. zip を展開し、中の `ngw.exe`・`LICENSE`・`LICENSE.ng` を好きなフォルダ
   （例：`%LOCALAPPDATA%\Programs\ngw`）に置く

自分でビルドする場合（Go 1.24 以上）：

```powershell
git clone https://github.com/tmhys/ngw
cd ngw
.\build.ps1
```

`dist\ngw.exe` ができます。Linux/macOS からのクロスビルドは `make`。

### 3. 起動する

`ngw.exe` をダブルクリックします。`nvim.exe` は PATH・winget・scoop・
`C:\Program Files\Neovim` から自動で探します。

- 初回は **「Windows によって PC が保護されました」**（SmartScreen）と出ることがあります。
  ngw.exe にはコード署名がないためです。「詳細情報」→「実行」で起動できます。
- ファイルを開くには、ファイルを ngw のウィンドウにドラッグ＆ドロップするか、
  エクスプローラーでファイルを右クリック →「プログラムから開く」→「別のアプリを選択」→ ngw.exe を選びます。
- コマンドラインから使うなら、置いたフォルダを PATH に追加します：

  ```powershell
  $dir = "$env:LOCALAPPDATA\Programs\ngw"
  [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path", "User") + ";$dir", "User")
  ```

  新しく開いた PowerShell で `ngw ファイル名` で起動できます。

## 使い方

```text
ngw [オプション] [--] [nvim の引数...]

  --nvim PATH            使う nvim.exe
  --server HOST:PORT     起動済みの "nvim --listen HOST:PORT" に接続
  --geometry COLSxROWS   起動時の桁数×行数（既定 100x32）
  --maximized            最大化して起動
```

それ以外の引数は nvim に渡ります（例：`ngw -O a.txt b.txt`）。

## 設定

設定ファイルはありません。普段の `init.lua` / `init.vim` に書きます。
ngw から起動したときは `g:ngw` が 1 になります。

```lua
if vim.g.ngw then
  vim.o.guifont = 'BIZ UDGothic:h12'      -- 既定。HackGen_Console_NF:h11 なども可
  -- vim.o.guifontwide = 'BIZ UDGothic'   -- 全角文字だけ別フォントにしたいとき
  -- vim.o.linespace = 2                  -- 行間（ピクセル）
  vim.o.ambiwidth = 'double'              -- ○※→ などを全角幅で扱う（日本語なら推奨）

  -- vim.g.ngw_ime_off_on_leave = 1  -- 挿入モードを抜けたら IME を必ずオフに
  -- vim.g.ngw_ime = false           -- IME を使える時を自分で決める（下の例）
  -- vim.g.ngw_ime_insert_only = 0   -- ノーマルモードでも IME を使う
  -- vim.g.ngw_clipboard = 0         -- 内蔵クリップボードを使わない
  -- vim.g.ngw_fullscreen = 1        -- 全画面（0 で戻る）
end
```

| 設定 | 意味 |
|---|---|
| `guifont` | `名前:h大きさ[:b][:i][:qCLEARTYPE\|qANTIALIASED\|qNONANTIALIASED]`、`,` 区切りで複数可。名前の空白は `_` でも可。`*` でダイアログ |
| `guifontwide` | 全角文字に優先して使うフォント |
| `linespace` | 行間 |
| `g:ngw_ime_insert_only` | 既定 1。文字入力モード以外では IME を切り離す |
| `g:ngw_ime_off_on_leave` | 既定 0。1 にすると挿入モードを抜けるたびに IME をオフ |
| `g:ngw_ime` | 未設定ならモードで自動。`true` / `false` を入れるとその通りに IME をつなぐ / 切る（切るときは状態を覚え、次に `true` にしたとき戻す）。Neovide の `g:neovide_input_ime` と同じ使い方 |
| `g:ngw_clipboard` | 既定 1。0 で内蔵プロバイダを無効化（`g:clipboard` を自分で設定した場合も無効） |
| `g:ngw_fullscreen` | 1 で全画面、0 で元に戻す |

`g:ngw_ime` の例（挿入モードと `/` `?` 検索のときだけ IME を使う）：

```lua
vim.g.ngw_ime = false
vim.api.nvim_create_autocmd({ "InsertEnter", "InsertLeave", "CmdlineEnter", "CmdlineLeave" }, {
  callback = function(args)
    if args.event:match("^Cmdline") and not vim.tbl_contains({ "/", "?" }, vim.fn.expand("<afile>")) then
      return
    end
    vim.g.ngw_ime = args.event:match("Enter$") ~= nil
  end,
})
```

キー操作：`Ctrl+Shift+V` でクリップボードを貼り付け（ターミナルと同じ。挿入・コマンドラインどちらでも）。
ウィンドウを閉じると `:confirm qall` 相当（未保存なら確認が出ます）。

## 構成

```text
cmd/ngw            起動・引数
internal/gui       ウィンドウ、描画（GDI）、IME、マウス、クリップボード  ← Windows 用に新規
internal/win32     Win32 API 宣言（syscall のみ、cgo なし）               ← 新規
internal/screen    グリッド状態（ng から。スクロールを blit にするフックを追加）
internal/input     キー表記変換（ng から。仮想キーコード対応を追加）
internal/nvim      nvim の起動と bootstrap Lua（ng から。stdio 接続・クリップボードを追加）
internal/rpc, msgpack, width   ng から流用
```

## 制限事項

- 色付き絵文字は GDI の制約で単色表示です。リガチャ（合字）は描画しません（ng と同じ）。
- ng と違い、GUI が落ちると Neovim も終了します（Windows では stdio の `--embed` 接続のため）。
  Neovim を残したい場合は `nvim --listen 127.0.0.1:6666` を別に起動して `ngw --server 127.0.0.1:6666`。
- `--server` は TCP のみ（名前付きパイプ未対応）。

## ライセンス

MIT（[LICENSE](LICENSE)）。ng（© Shougo Matsushita, MIT）由来部分の著作権表示と許諾文は
[LICENSE.ng](LICENSE.ng) にあります。ngw.exe を配布するときは、この 2 つのファイルを同梱してください。
