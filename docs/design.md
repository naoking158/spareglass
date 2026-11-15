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
    ├ media.Pipeline     [OS別]  capture → H.264 encode（未実装）
    └ WebRTC (Pion)      [共通]  H.264 video track + "control" DataChannel
  HTTP Server [共通]  web/ を embed で配信 + POST /api/session
```

## 層ごとの方針

### 仮想ディスプレイ（internal/vdisplay）

| OS | 手段 | 状態 | 注意 |
|---|---|---|---|
| macOS 14+ | CGVirtualDisplay（private API）。go-macos/virtualdisplay（purego, cgo 不要, BSD-3）を利用 | 実装済み（実機未検証） | App Store 配布不可。モード変更すると削除不能になるので「作り直し」で対応。名前ごとに ICC プロファイルが残るので名前は固定（spareglass N） |
| Windows 10/11 | Indirect Display Driver。自作せず署名済みの VirtualDrivers/Virtual-Display-Driver を前提にする | 未実装 | ドライバ更新時に黒画面になる事例あり。切断時に必ずモニタを外す。異常終了に備えてウォッチドッグ |
| Linux (Wayland) | xdg-desktop-portal ScreenCast の source type VIRTUAL（KDE）、GNOME は org.gnome.Mutter.ScreenCast.Session.RecordVirtual を直接 | 未実装 | DE ごとの成熟度差が大きい（KDE の解像度固定バグ、GNOME 50 の更新遅延報告など）。正式サポート範囲を明示する |
| Linux (X11) | xrandr の仮想出力 | 未実装 | フォールバック扱い |

### キャプチャ＋エンコード（internal/media）

- darwin: ScreenCaptureKit → VideoToolbox
- windows: Windows.Graphics.Capture → Media Foundation（または NVENC/AMF/QSV）
- linux: PipeWire（portal の node）→ VA-API / NVENC
- キャプチャからエンコードまで GPU メモリのまま渡す（IOSurface / D3D11 / DMA-BUF）。
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

### ペアリング・セキュリティ（internal/pairing）

- 起動ごとにランダムトークン（設定で固定も可）。URL フラグメント `#t=` に入れ、リクエスト行やログに出さない。
- `/api/session` は Bearer トークン必須。メディアは DTLS-SRTP で暗号化。
- TODO: 新規端末の接続をホスト側で承認する UI。

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
- 現在は全て pure Go（CGO_ENABLED=0）なので Linux ランナー 1 台でクロスコンパイルできる。
- media 層で cgo を入れたら、OS 別ランナー（macos / windows / ubuntu）でビルドし、最後のジョブで Release にまとめる構成に変える。
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
