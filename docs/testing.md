# Testes

## Rodando

```sh
go test ./...
```

Roda em qualquer SO (Linux, Windows) — os pacotes com teste automatizado
hoje não dependem de nenhuma API específica de plataforma. Um CI em Linux
consegue rodar a suíte inteira normalmente, mesmo sem conseguir *compilar*
os arquivos `_windows.go` (que ficam de fora do build no Linux por
`//go:build windows` — ver abaixo).

## O que tem teste automatizado

Pacotes/arquivos sem dependência de SO, cobertos com teste unitário:

| Pacote/arquivo | O que testa |
|---|---|
| `internal/protocol` | round-trip de `WriteMessage`/`ReadMessage`, limite de tamanho, mensagem truncada, `Edge.Opposite()` |
| `internal/clipsync` | round-trip de `WriteFrame`/`ReadFrame`, limite de tamanho, `ClipAddr` (derivação porta+1) |
| `internal/auth` | `TokensEqual` (igual, diferente, tamanho diferente, vazio) |
| `internal/tlsutil` | persistência de certificado, formato/determinismo do fingerprint, `KnownHosts` (trust/lookup/persistência em disco) |
| `internal/keys` | round-trip `NameToVK`↔`VKToName` (Windows) / `NameToKeycode`↔`KeycodeToName` (Linux) sem código duplicado — só roda a metade que bate com o SO atual |
| `cmd/controller/edge_math.go` | toda a geometria pura da troca por borda (`entryPosition`, `pushesPast`, `hasMovedAway`, `releaseRelPos`, `controllerWarpPosition`, mais um teste de round-trip engage→disengage) — extraída de `edge_windows.go` **de propósito** pra não precisar de Windows pra testar. É a mesma matemática por trás dos dois bugs de mouse descritos em [`docs/known-issues.md`](known-issues.md). |

## O que **não** tem, e por quê

O grosso da lógica que realmente importa (hooks de baixo nível do
Windows, `SendInput`, Raw Input, leitura/escrita do clipboard nativo) vive
em arquivos `_windows.go` que chamam a API do Win32 direto via `syscall`.
Não dá pra unit-testar isso de verdade sem uma sessão Windows real com
teclado/mouse/clipboard — não existe um jeito barato de simular
`WH_MOUSE_LL`/`SendInput`/`OpenClipboard` em teste automatizado, e mockar
tudo isso perderia justamente a parte que mais quebrou até hoje (ver os
bugs em [`docs/known-issues.md`](known-issues.md) — nenhum deles teria
sido pego por um mock).

Regra usada neste repo: **toda lógica pura (matemática, parsing, framing,
decisão) que puder ser separada da chamada de sistema deve ser extraída
pra um arquivo sem build tag e testada** (é o que foi feito com
`edge_math.go` e é o modelo pra próximas extrações — ex. se a lógica de
parsing de `BI_BITFIELDS` em `internal/clipboard` crescer, vale separar o
parsing puro de bytes→`image.Image` da chamada de `GetClipboardData`).

### Checklist de teste manual (Windows-only, duas máquinas)

Sem substituto automatizado pra isso hoje — rodar depois de qualquer
mudança em `internal/capture`, `internal/inject`, `internal/clipboard` ou
`cmd/*/edge_windows.go`/`cmd/*/clipboard_windows.go`:

1. **Legado**: `controller` sem `-edge` reflete teclado/mouse continuamente.
2. **Edge crossing**: empurrar o cursor pra borda engaja; empurrar de
   volta pela mesma borda desengaja — sem flip espúrio logo após engajar
   (era o bug do `pt`-diffing).
3. **Clipboard texto**, as duas direções (`Ctrl+Alt+V` engaged e
   disengaged).
4. **Clipboard imagem** (`PrintScreen`), as duas direções — cobre o
   parsing de `BI_BITFIELDS`.
5. **Segurar o atalho por mais tempo** (autorepeat do `V`) não deve deixar
   nenhum modificador preso depois.
6. Se mexer em `internal/keys` ou nos mapeamentos de tecla: testar pelo
   menos uma tecla de cada categoria (letra, número, função, modificador,
   pontuação) chegando certa do outro lado.

## `scripts/build.sh`

Não é teste, mas é o que garante que os quatro alvos de build (target e
controller pra Windows e Linux, tray só Windows) continuam compilando —
rode antes de abrir PR:

```sh
./scripts/build.sh
```
