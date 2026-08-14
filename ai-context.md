# ai-context

Ponto de entrada pra quem (ou qual IA) for mexer no `kbs` no dia a dia.
Este arquivo fica **enxuto de propósito** — é um índice, não uma
enciclopédia. Só abra os arquivos linkados abaixo quando a tarefa em mãos
realmente tocar naquela área; não carregue tudo sempre.

## O que é

`kbs` é um "reverse KVM": `controller` captura teclado/mouse local e manda
pro `target`, que injeta. Dois modos (sempre-compartilha, ou troca por
borda tipo Synergy via `-edge`), clipboard compartilhado via `Ctrl+Alt+V`,
e um ícone de bandeja (`tray`) opcional pra rodar sem terminal. Detalhes
de uso: [`README.md`](README.md).

Go, sem CGO, Windows é a plataforma principal (todo o input real
acontece via `syscall` direto no Win32); Linux tem só o modo
sempre-compartilha; macOS não implementado.

## Antes de mexer em algo, leia

| Se a tarefa envolve... | Leia |
|---|---|
| Entender como as peças se conectam, protocolo, estados de engage/disengage | [`docs/architecture.md`](docs/architecture.md) |
| Um bug que *parece* familiar (mouse trava, clipboard não cola, tecla presa) | [`docs/known-issues.md`](docs/known-issues.md) — **confira aqui antes de investigar do zero** |
| Rodar ou escrever teste, decidir se algo é testável | [`docs/testing.md`](docs/testing.md) |
| Buildar pra Windows/Linux | [`scripts/build.sh`](scripts/build.sh) |

## Os 5 fatos que mais custam redescobrir

1. **Nunca faça diff de posição absoluta do cursor (`pt` do hook) através
   de um trecho onde o input está sendo suprimido/injetado** — o Windows
   não confirma a posição nesse caso, e o diff vira ruído. Use Raw Input
   (`WM_INPUT`/`RAWMOUSE`) pra delta relativo de verdade. (bug real, ver
   known-issues.md)
2. **`Ctrl+Alt+V` só suprime o `V`** — Ctrl/Alt continuam sendo
   encaminhados/injetados como teclas normais. Qualquer injeção de tecla
   sintética que dependa de um modificador "limpo" precisa soltar
   Ctrl/Alt/Shift primeiro.
3. **Imagem de clipboard do Windows quase sempre vem como
   `BI_BITFIELDS`**, não `BI_RGB` — não assuma o formato mais simples.
4. **Teclas normais (não-modificadoras) têm autorepeat do Windows**; um
   handler de hotkey precisa deduplicar por pressão física, não por
   evento de tecla, ou dispara múltiplas vezes concorrentes enquanto
   segurada.
5. **Janelas/processos elevados (UAC, antivírus) não respondem a
   `SendInput` de um processo com privilégio menor** — é o Windows
   (UIPI), não um bug nosso; a mesma limitação existe em Synergy/Barrier.

## Lógica pura vs. syscall — onde vive o quê

Regra do repo: lógica que não depende de SO fica em arquivo **sem** build
tag (testável em qualquer plataforma); o que chama API de SO fica isolado
em `_windows.go`/`_linux.go`/`_other.go` com a mesma assinatura de função
nos dois lados. Exemplo canônico: a matemática de troca de borda vive em
`cmd/controller/edge_math.go` (sem build tag, testada), separada do hook
do Windows em `edge_windows.go`. Ao adicionar lógica nova com uma
chamada de sistema no meio, considere se a decisão/parsing pode ser
extraída do mesmo jeito.

## Convenção de build paralelo

Ao testar uma mudança que pode quebrar uma sessão já em uso (duas
máquinas conectadas ao vivo), builde sob um sufixo em vez de sobrescrever
o binário em uso: `./scripts/build.sh --suffix 2` gera
`target2.exe`/`controller2.exe`/`tray2.exe` ao lado dos originais. Não é
uma convenção permanente do projeto — é só pra não derrubar quem já está
conectado enquanto se testa algo novo.
