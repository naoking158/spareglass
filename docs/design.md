# spareglass 設計メモ

ブラウザを持つ任意の端末を、メイン端末（ホスト）の拡張ディスプレイとして使えるようにする。
受信側はブラウザのみ、ホストは単一バイナリで動かすことを目標にする。

## 全体構成

```
Host (Go 単一バイナリ)
  Session Manager ── クライアント1台につき1セッション・1仮想ディスプレイ
    ├ vdisplay.Provider  [OS別]  darwin: CGVirtualDisplay
    │                            windows: IddCx (VDD を制御)
    │                            linux: portal VIRTUAL / Mutter RecordVirtual
    ├ media.Pipeline     [OS別]  capture → H.264 encode
    │                            darwin: ScreenCaptureKit → VideoToolbox（cgo）
    │                            windows: ffmpeg（gdigrab + Media Foundation）
    └ WebRTC (Pion)      [共通]  H.264 video track + "control" DataChannel
  HTTP Server [共通]  web/ を embed で配信 + POST /api/session
```

## 層ごとの方針

### 仮想ディスプレイ（internal/vdisplay）

| OS | 手段 | 状態 | 注意 |
|---|---|---|---|
| macOS 14+ | CGVirtualDisplay（private API）。go-macos/virtualdisplay（purego, cgo 不要, BSD-3）を利用 | 実装済み | App Store 配布不可。モード変更すると削除不能になるので「作り直し」で対応。名前ごとに ICC プロファイルが残るので名前は固定（spareglass N） |
| Windows 10/11 | Indirect Display Driver。自作せず署名済みの VirtualDrivers/Virtual-Display-Driver を前提にする | 未実装 | ドライバ更新時に黒画面になる事例あり。切断時に必ずモニタを外す。異常終了に備えてウォッチドッグ |
| Linux (Wayland) | xdg-desktop-portal ScreenCast の source type VIRTUAL（KDE）、GNOME は org.gnome.Mutter.ScreenCast.Session.RecordVirtual を直接 | 未実装 | DE ごとの成熟度差が大きい（KDE の解像度固定バグ、GNOME 50 の更新遅延報告など）。正式サポート範囲を明示する |
| Linux (X11) | xrandr の仮想出力 | 未実装 | フォールバック扱い |

**macOS の HiDPI**: クライアントの DPR が `2 × render_scale` 以上なら、CSS サイズ（pt）の 2 倍の画素を持つ HiDPI ディスプレイを作る。

- モード切替はしない。最大モードを画素サイズ、HiDPI フラグ、追加モードを pt サイズ、物理サイズを 220dpi 相当にして作ると、macOS は最初から pt サイズ・2 倍の画素で起動する。
- 小さいサイズでは等倍で起動することがある。CGDisplayBounds（pt）で確かめ、Retina でなければ作り直して等倍で開く。
- go-macos/virtualdisplay は、作ったプロセスからモードを読めないと pt を画素として比べ、Retina での起動を ErrWrongMode で弾く。v0.5.0 を `internal/virtualdisplay` に取り込み、go.mod の replace で使って、その判定だけ直している（`PATCHES.md`）。
- 画素数は 3840×2400 までにする。VideoToolbox の H.264 は 5120×2880 で開けない。これを超えるクライアントは等倍のまま。
- キャプチャする画素サイズは、ディスプレイを作った session から media に渡す。同じプロセスで作った 2 枚目以降の HiDPI ディスプレイについて、ScreenCaptureKit が pt のサイズを返すため。
- 文字は鮮明になる代わりに遅延が増える。1920x1080 pt（loopback）で、pointer warp からホスト側の最終パケットまで約 35ms → 約 55ms。

### キャプチャ＋エンコード（internal/media）

**macOS（cgo ビルド、配布物を含む）**: ScreenCaptureKit と VideoToolbox をプロセス内で使う（`native_darwin.m`）。

- SCStream で仮想ディスプレイを BGRA で受け取る。YUV で出させると画面より約 8/255 明るくなるため、YUV への変換は VideoToolbox に任せる。
- VideoToolbox は RealTime、Constrained Baseline、フレーム並べ替えなし、709 の色タグ。エンコード結果は 1 フレームずつすぐ出る。
- ScreenCaptureKit は画面が変わったときだけフレームを出す。PLI を受けたら最後のフレームをキーフレームとして再エンコードする。キーフレーム間隔の上限は 10 秒。
- 必要なのは画面収録権限だけ。1920x1080 で pointer warp からブラウザ表示まで約 40ms（loopback）。
- `fps` を 120 にすると仮想ディスプレイも 120Hz で作られ、ScreenCaptureKit の最小間隔も 1/120 になる。1920x1080 の等倍では、pointer warp からホスト側の最終パケットまでが約 40ms → 約 29ms（loopback）。HiDPI では差が計測のぶれに埋もれる。既定は 60 のまま。動いている間の帯域が増え（目標 12Mbps に対し 5 秒平均で 17.6Mbps の区間があった）、`fps` は Windows の表示モードと ffmpeg にも効くが、そちらは未計測のため。

**macOS（cgo なし）と Windows**: ffmpeg を子プロセスで起動する。macOS は `-f avfoundation -i "Capture screen N"`（N は CGGetActiveDisplayList 上の位置）→ `h264_videotoolbox`、Windows は gdigrab → Media Foundation。いずれも Constrained Baseline、B-frame なし、GOP 1 秒、IDR ごとに SPS/PPS。stdout の Annex-B を access unit 単位に分割して Pion に渡す。
制約: access unit の区切りが次フレーム到着で確定するため最大 1 フレーム遅延、PLI でキーフレームを要求できない（GOP 1 秒で代替）、ffmpeg が必要。1920x1080 の macOS では、ホスト側の遅延がプロセス内の経路より約 110ms 大きい（146ms 対 37ms）。
`serve --test-pattern` で ffmpeg のテストパターンを流せる（ブラウザ側の切り分け用、どの OS でも ffmpeg を使う）。

**次段**:

- windows: Windows.Graphics.Capture → Media Foundation（または NVENC/AMF/QSV）
- linux: PipeWire（portal の node）→ VA-API / NVENC
- キャプチャからエンコードまで GPU メモリのまま渡す（D3D11 / DMA-BUF）。
- H.264、B-frame なし、低遅延プリセット、ブラウザからの PLI/FIR でキーフレーム。
- いずれも cgo がほぼ必須。ここが Go の試行錯誤が一番重くなる部分。

### 配信（internal/server, web/）

- WebRTC（Pion）。RTCPeerConnection はセキュアコンテキスト不要なので LAN の HTTP のままで動く。
  WebSocket + WebCodecs 案は HTTPS 必須になるため不採用。
- シグナリングは非 trickle の 1 往復（WHEP に近い）: ブラウザが offer を POST し、ICE 収集済みの answer を返す。
- STUN/TURN なし（LAN 前提）。ブラウザの mDNS 候補は Pion が解決する。
- control DataChannel でクライアントの画面サイズ・DPR・向きを受け取り、そのサイズで仮想ディスプレイを作る。
  サイズ変更時は作り直し。
- クライアントは `jitterBufferTarget = 0` で遅延を詰める。
- RTP timestamp は各フレーム自身のキャプチャ時刻にする。1 つ前のフレームの時刻を付けると、間隔が不規則なキャプチャでブラウザの jitter buffer が伸びる。
- ブラウザが playout-delay 拡張（`http://www.webrtc.org/experiments/rtp-hdrext/playout-delay`）に対応していれば min=max=0 を付ける。画面が変わったときだけ送るフレームを Chrome が数百 ms 抱えるのを防ぐ。

### URL とペアリング（internal/pairing）

- URL は手打ち前提で短くする: `http://<ホストIP>/<ディスプレイ番号>`。ポートは既定で 80（macOS / Windows は一般ユーザーで可）、使えなければ 7878。
- `.local` 名（mDNS）は今回は見送り。素の `foo-bar` はルーター DNS か hosts 設定が必要なためアプリ単体では不可。
- 初回: ブラウザが `POST /api/pair` → 4 桁コードを表示 → ホストの操作画面に同じコードと「許可する／許可しない」を出す。
  ターミナルの「許可しますか? [y/N]」も残し、先に答えた方を採る（操作画面で答えるとターミナルの問い合わせは取り消す）。
  ブラウザは `GET /api/pair/{id}` をポーリングし、承認されたら device id と secret を一度だけ受け取って localStorage に保存。
- 2 回目以降: `Authorization: Device <id>:<secret>` で `POST /api/session`。ホストは secret の SHA-256 だけを `devices.json` に保存。
- 未応答のリクエストは 2 分で失効、同時待ちは最大 4 件。
- 許可した機器は操作画面の「削除」か `spareglass devices [list|revoke ID|clear]` で消す。ホストはファイル更新を検知して再読込するので、起動中でも即反映。
  操作画面から消すと、その機器の接続中のセッションも閉じる。
- 承認の 1 タップではフルスクリーンにできない（ユーザー操作が非同期処理で失われる）ため、承認後にもう一度 Connect を押す。

### ホストの操作画面（internal/server/host.go, web/host.*）

- ターミナルを使わない人向けに、接続の許可、許可した機器の削除、終了をブラウザの画面でできるようにする。
  専用アプリにはせず、ホストが配信用とは別に `127.0.0.1:0` で待ち受け、起動時に既定のブラウザで開く（`--no-open` で開かない）。
  Windows 向けのビルドを変えずに済み、検証もブラウザ自動操作で回せる。
- 画面: タブレットで開く URL と QR コード、許可待ちの要求、接続中の画面、許可した機器、終了ボタン。
  文言はブラウザの言語で日本語か英語。1 秒ごとに `GET /api/state` をポーリングし、変わった区画だけ描き直す。
- API: `GET /api/state`、`POST /api/pair/{id}`（`{"allow":bool}`）、`DELETE /api/devices/{id}`、`POST /api/stop`。
  終了は Ctrl+C と同じ後始末の経路を通る。
- 守り: 接続元が loopback、Host が `127.0.0.1` か `localhost`（DNS rebinding 対策）、Origin が一致（CSRF 対策）。
  API は起動ごとの乱数トークンを `X-Spareglass-Token` ヘッダで要求する。トークンは URL の `#` 以降に載せ、HTTP リクエストのログや Referer に出さない。
- 再び開くときは、ターミナルに出る `Host page` の URL を `#` 以降まで含めて使う。

## 制御メッセージ（DataChannel "control"）

Client → Host

```json
{"type":"hello","width":1180,"height":820,"dpr":2,"orientation":"landscape-primary"}
{"type":"resize", ...同上}
```

Host → Client

```json
{"type":"status","state":"streaming|display-only|no-display|error","detail":"...","width":1180,"height":820}
```

将来: `{"type":"pointer", ...}` でタッチ／ポインタ入力（macOS: CGEvent, Windows: SendInput, Linux: RemoteDesktop portal）。

## 配布（Release）

- タグ `v*` を push すると GitHub Actions で GoReleaser が動き、darwin/windows/linux × amd64/arm64 のアーカイブを Release に添付する。
- macOS は cgo（ScreenCaptureKit / VideoToolbox）でビルドするため、GoReleaser は macOS ランナーで動かす。`MACOSX_DEPLOYMENT_TARGET=14.0` の universal binary。Windows と Linux は CGO_ENABLED=0 で同じランナーからクロスコンパイルする。
- Windows の media 層で cgo を入れたら、Windows ランナーでのビルドを足す。
- macOS は未公証だと Gatekeeper に止められる。将来は Developer ID 署名＋公証、Homebrew tap、winget も検討。

## マイルストーン

1. **Phase 0**: macOS で 仮想ディスプレイ → ScreenCaptureKit → VideoToolbox → WebRTC → ブラウザ表示（縦に 1 本通す）
2. **Phase 1**: 接続承認 UI、切断時の後始末の堅牢化、複数端末、自動再接続
3. **Phase 2**: Windows（VDD 制御 + WGC + HW エンコード）
4. **Phase 3**: Linux（GNOME / KDE / X11）
5. **Phase 4**: タッチ入力、音声

## 参考

- Deskreen: ブラウザを第 2 画面にする Electron + WebRTC アプリ。拡張にはダミープラグが必要（AGPL）。
- OpenDisplay: Mac 向け。CGVirtualDisplay + ScreenCaptureKit + VideoToolbox（現行版はコピーレフト）。
- go-macos/virtualdisplay: CGVirtualDisplay の Go バインディング（BSD-3）。
- VirtualDrivers/Virtual-Display-Driver: Windows 用 IddCx 仮想ディスプレイドライバ。
- xdg-desktop-portal ScreenCast: source type 4 = VIRTUAL。
- Mutter: org.gnome.Mutter.ScreenCast.Session.RecordVirtual。
