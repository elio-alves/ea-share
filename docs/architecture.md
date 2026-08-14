# Arquitetura

## Visão geral

```
controller (captura input local)  --TLS-->  target (injeta input recebido)
      |                                            |
      capture.Source                          inject.Injector
      (WH_*_LL hooks + Raw Input, Windows)     (SendInput, Windows)
```

Dois binários (`cmd/controller`, `cmd/target`), um pacote de perfis/GUI
(`cmd/tray`), conectados por dois protocolos de rede independentes:

- **`internal/protocol`** — mouse/teclado/handshake, latência importa.
- **`internal/clipsync`** — clipboard (texto/imagem), pode ser grande e
  lento sem afetar o outro.

Cada `cmd/*` tem arquivos `_windows.go` / `_other.go` (ou `_linux.go`) pra
isolar o que só existe numa plataforma — o `main.go` de cada um é
agnóstico de SO e só chama funções com a mesma assinatura dos dois lados
(ex. `runEdgeAware`, `startClipboardListener`).

## Dois modos do `controller`

- **Legado** (sem `-edge`): `capture.New()` — encaminha tudo sempre,
  sem supressão local. `cmd/controller/main.go:runLegacy`.
- **Edge-aware** (`-edge left|right|top|bottom`): `capture.NewEdgeAware`
  — só encaminha/suprime depois que o cursor cruza a borda configurada.
  `cmd/controller/edge_windows.go:runEdgeAware`.

## Máquina de estados do engage/disengage

Vive em `runEdgeAware` (`cmd/controller/edge_windows.go`), com a
geometria pura extraída pra `cmd/controller/edge_math.go` (sem build tag,
testável em qualquer SO — ver [`docs/testing.md`](testing.md)).

1. Cursor cruza a borda configurada → `capture.EdgeCrossedEvent` →
   `engaged = true`, manda `MsgEngage` (com a posição relativa ao longo da
   borda), começa a simular a posição do cursor no `target` (`vx, vy`)
   localmente, sem precisar de round-trip de rede pra saber quando soltar.
2. Enquanto engaged, cada `MouseMoveEvent` atualiza `vx, vy` e checa se
   ele já se afastou da borda de entrada (`hasMovedAway`) e, depois, se
   empurrou de volta pra fora dela (`pushesPast`) — só conta como
   "soltar" se as duas coisas aconteceram nessa ordem (`movedAway &&
   pushingOutEntry`), pra não soltar sozinho por causa de ruído logo após
   o crossing.
3. Ao soltar: calcula onde o cursor *local* deve reaparecer
   (`controllerWarpPosition`, um pixel pra dentro da borda que disparou o
   crossing) e chama `src.Disengage(...)`.

O `target` só participa passivamente: ao receber `MsgEngage`, calcula a
posição de entrada (mesma lógica, `entryPosition`) e faz um único
`SetCursorPos` — dali em diante só recebe `MsgMouseMove` (deltas) até o
próximo `MsgEngage`.

## Clipboard (`Ctrl+Alt+V`)

- **Detecção do atalho**: `internal/capture/capture_windows.go`,
  `keyboardProc` rastreia estado de Ctrl/Alt segurados *fora* do portão
  normal de forward/suppress (funciona engaged ou disengaged). Ao ver `V`
  descendo com os dois segurados, emite `HotkeyPasteEvent` e suprime a
  tecla — nunca é encaminhada como `V` literal, nem local nem pro target.
- **Conexão dedicada**: `internal/clipsync` — framing binário simples
  (kind + tamanho + payload, sem JSON/base64) numa segunda conexão TLS,
  porta principal + 1 (`ClipAddr`). Existe pra um payload grande (print de
  tela) nunca competir na fila com pacotes de mouse na conexão principal.
- **Direção**: decidida em `cmd/controller/edge_windows.go` no momento do
  `HotkeyPasteEvent`, lendo o `engaged` atual — engaged empurra o
  clipboard local pro target; disengaged pede o do target e cola local
  (por isso o `controller` tem seu próprio `inject.New()`, que ele não
  precisava antes desse recurso).
- **Leitura/escrita do clipboard do Windows**: `internal/clipboard` — só
  texto (`CF_UNICODETEXT`) e imagem (`CF_DIB` ↔ PNG, parsing manual de
  `BITMAPINFOHEADER`, ver [`docs/known-issues.md`](known-issues.md) sobre
  `BI_BITFIELDS`).

## `tray`

`cmd/tray` não tem lógica de captura/injeção própria — é só uma casca de
processo: lê/escreve `%AppData%\kbs\tray_profiles.json`, desenha o menu
(`fyne.io/systray`) e sobe `target*.exe`/`controller*.exe` como
subprocessos escondidos (`CREATE_NO_WINDOW`), capturando a saída deles
pro log. Ver [`docs/known-issues.md`](known-issues.md) sobre a lacuna de
log do próprio `tray.exe`.

## Convenção de versionamento pra desenvolvimento

Quando uma mudança pode quebrar uma sessão já em uso nas duas máquinas,
builda-se sob um nome diferente (`target2.exe`, `controller2.exe`,
`tray2.exe`, via `scripts/build.sh --suffix 2`) em vez de sobrescrever o
que já está rodando — assim dá pra testar sem derrubar quem já está
conectado. `cmd/tray` não hardcoda esse sufixo; é uma prática de deploy
paralelo, não uma feature do software.
