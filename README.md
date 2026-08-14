# kbs — reverse KVM

Compartilhamento de teclado/mouse "ao contrário": em vez da máquina que
hospeda o teclado físico atuar como servidor (Synergy/Barrier), aqui é quem
**conecta** que compartilha o teclado/mouse.

- **`target`** — a máquina que é controlada. Fica escutando, aceita uma
  conexão autenticada e injeta os eventos recebidos localmente.
- **`controller`** — a máquina controladora. Conecta no `target`, captura o
  teclado/mouse *local* e envia os eventos pela rede.
- **`tray`** — ícone na bandeja do Windows pra iniciar `target`/`controller`
  a partir de perfis salvos, sem precisar de terminal (veja
  [Ícone na bandeja (`tray`)](#ícone-na-bandeja-tray)).

## Uso responsável

Isso é uma ferramenta de acesso remoto — use só entre máquinas suas (ou com
autorização explícita do dono), do mesmo jeito que usaria SSH, TeamViewer ou
Synergy. Quem tiver o token consegue digitar e clicar na máquina `target`
sem mais nenhuma confirmação; trate-o como uma senha. Veja
[Modelo de segurança](#modelo-de-segurança) antes de expor isso numa rede
que não controla.

## Build

```sh
go build -o bin/target.exe ./cmd/target        # Windows
go build -o bin/controller.exe ./cmd/controller
go build -ldflags "-H=windowsgui" -o bin/tray.exe ./cmd/tray   # sem console ao abrir

GOOS=linux GOARCH=amd64 go build -o bin/target ./cmd/target   # cross-compile p/ Linux
GOOS=linux GOARCH=amd64 go build -o bin/controller ./cmd/controller
```

Ou use `scripts/build.sh` (funciona rodando de Windows via Git Bash ou de
Linux/CI — veja [`scripts/build.sh`](scripts/build.sh)):

```sh
./scripts/build.sh              # target/controller (windows+linux) + tray (windows)
./scripts/build.sh --suffix 2   # gera target2.exe/controller2.exe/tray2.exe, sem sobrescrever um build já em uso
```

Nenhuma dependência usa CGO — não precisa de gcc/mingw instalado para
compilar para Windows ou Linux, nem para cross-compilar de um SO pro outro.
macOS **não é suportado ainda** (veja [Limitações](#limitações-conhecidas)).

## Uso

Na máquina que vai ser controlada (`target`):

```sh
./target -listen :7777
```

Ele gera (na primeira execução) um certificado autoassinado e, se você não
passar `-token`, um token aleatório — ambos são impressos no terminal:

```
No -token given; generated one for this session:
  4fa94c92cf6e96a878652410cb58fdb2b769beb7b88ac859

Certificate fingerprint (verify this matches on the controller):
  D4:83:C3:D3:DB:17:4A:57:...
```

Na máquina controladora (`controller`), copiando o token mostrado acima:

```sh
./controller -connect 192.168.1.50:7777 -token 4fa94c92cf6e96a878652410cb58fdb2b769beb7b88ac859
```

Na primeira conexão a um `target` desconhecido, o `controller` mostra o
fingerprint do certificado e pede confirmação (como o `known_hosts` do SSH)
antes de confiar nele. Depois disso, esse fingerprint fica salvo e é
conferido automaticamente em toda reconexão — se mudar, a conexão é
recusada (proteção contra man-in-the-middle).

Sem `-edge`, teclado e mouse do `controller` passam a ser refletidos
*sempre* no `target` (modo antigo, "sempre compartilha"). `Ctrl+C` encerra
dos dois lados.

### Troca por borda de tela (`-edge`)

Com `-edge left|right|top|bottom`, o `controller` se comporta como o
Synergy/Barrier: o teclado/mouse local funciona normalmente até o cursor
encostar na borda configurada da tela — a partir daí ele passa a ser
capturado e encaminhado pro `target`, com o cursor "entrando" pela borda
oposta de lá. Empurrar de volta pela mesma borda devolve o controle local.

```sh
./controller -connect 192.168.1.50:7777 -token ... -edge right   # target fica à direita, no mundo físico
```

`-edge` é o lado da tela **onde o target fica**, do ponto de vista de quem
está sentado no `controller`.

### Clipboard compartilhado (`Ctrl+Alt+V`)

Com `-edge` ativo, `Ctrl+Alt+V` transfere o clipboard (texto ou imagem, ex.
print de tela) entre as duas máquinas — a direção é decidida automaticamente
pelo estado atual:

- **Controlando o target** (engaged) → `Ctrl+Alt+V` empurra o clipboard do
  `controller` pro `target` e cola lá.
- **Controle local** (disengaged) → `Ctrl+Alt+V` busca o clipboard do
  `target` e cola aqui no `controller`.

`Ctrl+C`/`Ctrl+V` comuns continuam funcionando normalmente em cada máquina
(a tecla é só encaminhada como qualquer outra) — o atalho especial só troca
o *conteúdo do clipboard* entre as duas pontas antes de colar. Usa uma
conexão TCP separada da de mouse/teclado (uma porta acima), então um print
de tela grande nunca atrasa o movimento do mouse.

### Flags principais

| Flag (target) | Descrição |
|---|---|
| `-listen` | endereço:porta para escutar (padrão `:7777`) |
| `-token` | segredo compartilhado (ou env `KBS_TOKEN`); gerado se vazio |
| `-data-dir` | onde guardar o certificado TLS |
| `-verbose` | loga periodicamente quantos eventos de cada tipo foram recebidos |

| Flag (controller) | Descrição |
|---|---|
| `-connect` | `host:porta` do target (obrigatório) |
| `-token` | segredo compartilhado (ou env `KBS_TOKEN`) |
| `-edge` | `left\|right\|top\|bottom`: ativa troca por borda + clipboard; sem isso, compartilha sempre (modo antigo) |
| `-fingerprint` | fixa o fingerprint esperado do target, pulando o prompt |
| `-yes` | confia automaticamente num target desconhecido, sem perguntar |
| `-known-hosts` | caminho do arquivo de fingerprints confiados |

## Modelo de segurança

- A conexão é sempre TLS. Como não há uma CA, a confiança é por
  **fingerprint pinning** (trust-on-first-use), igual ao SSH — não por uma
  cadeia de certificado validada. O canal de clipboard (quando `-edge` está
  ativo) é uma segunda conexão TLS pro mesmo destino, com o fingerprint já
  confiado na primeira — não é uma decisão de confiança separada.
- Depois do handshake TLS, o `controller` precisa enviar o **token**
  correto antes do `target` aceitar qualquer evento de input. Comparação é
  em tempo constante.
- O `target` só aceita **um controlador por vez**; conexões extras são
  recusadas enquanto uma já está ativa.
- **Trate o token como uma senha root**: quem o tiver pode digitar e
  clicar remotamente na máquina `target` sem mais nenhuma confirmação.
  Não hardcode o token em scripts compartilhados; prefira passá-lo via
  `KBS_TOKEN` no ambiente.
- Em rede não confiável (internet aberta), rode isso atrás de uma VPN
  (WireGuard/Tailscale) em vez de expor a porta diretamente.

## Permissões necessárias

- **Windows**: nenhuma permissão especial para captura/injeção via
  `SetWindowsHookExW`/`SendInput` em uso normal. Ver
  [Limitações](#limitações-conhecidas) sobre janelas/processos elevados
  (ex. antivírus) — nesse caso rodar como Administrador ajuda em parte.
- **Linux**:
  - `controller` (captura): precisa ler `/dev/input/event*`, o que
    normalmente exige root ou fazer parte do grupo `input`
    (`sudo usermod -aG input $USER`, depois reabrir a sessão).
  - `target` (injeção): precisa de acesso a `/dev/uinput`
    (`sudo modprobe uinput`; regra `udev` ou grupo `uinput`/`input`
    conforme a distro).
  - `-edge` e o clipboard compartilhado ainda não têm implementação no
    Linux (ver [Limitações](#limitações-conhecidas)).

## Ícone na bandeja (`tray`)

`tray.exe` é um jeito gráfico de usar o `target`/`controller` sem abrir
terminal: um ícone fica na bandeja do Windows (perto do relógio) com um
menu pra iniciar/parar cada um a partir de **perfis salvos**.

- Na primeira execução, `tray.exe` cria
  `%AppData%\kbs\tray_profiles.json` com um perfil de exemplo de cada tipo.
  Edite esse arquivo (menu **Editar perfis (notepad)**, depois **Recarregar
  perfis**) pra adicionar as suas máquinas: `name`, `listen`/`token` pros
  targets; `name`, `connect`/`token`/`edge` pros controllers (`edge` vazio
  = modo antigo, sempre compartilha, sem troca por borda).
- **Ouvir como target** / **Conectar em** no menu iniciam o processo
  correspondente (escondido, sem janela de console) usando `target.exe` /
  `controller.exe` — que precisam estar **na mesma pasta** que `tray.exe`.
- **Parar target** / **Parar controller** encerram o processo em execução.
- **Copiar token do target** copia pra área de transferência (pra colar na
  hora de criar o perfil do controller na outra máquina).
- Conexões feitas pelo `tray` confiam automaticamente no certificado de um
  target desconhecido na primeira vez (equivalente a `-yes`, já que não há
  terminal pra confirmar o fingerprint) — depois disso o fingerprint fica
  fixado normalmente, então uma reinstalação do target ou um MITM real
  ainda derruba a conexão.
- Log de diagnóstico (saída de cada `target`/`controller` iniciado pelo
  tray) fica em `%AppData%\kbs\tray.log`.

## Limitações conhecidas

- **macOS não implementado.** Captura/injeção no macOS exigem CGO
  (Quartz `CGEventTap`/`CGEventPost`), fora do escopo desta primeira
  versão que roda sem toolchain C.
- **`-edge` e clipboard compartilhado são Windows-only.** No Linux, o
  `controller`/`target` funcionam só no modo antigo (sempre compartilha).
- **Só um sentido por vez.** O `target` também não devolve o teclado/mouse
  dele para o `controller` — a ideia é dar suporte aos dois sentidos depois.
- **Sem bloqueio do input local do alvo.** Enquanto controlado, o `target`
  continua respondendo ao teclado/mouse físico dele também (podem colidir).
- **Layout de teclado**: o conjunto de teclas mapeado cobre letras,
  números, função (F1-F12), navegação, modificadores e pontuação comum de
  layout US. Teclas de acentuação/idiomas específicos (ç, á, teclas mortas
  etc.) não têm mapeamento ainda.
- Sem suporte a arrastar-arquivos ou múltiplos monitores/displays — é só
  teclado, mouse (movimento relativo, botões e scroll vertical) e o
  clipboard (texto/imagem) descrito acima.
- **Janelas/processos elevados no Windows (ex. ícone de antivírus na
  bandeja) podem não responder ao input injetado.** É uma proteção do
  próprio Windows (UIPI) contra input sintético de um processo com
  privilégio menor afetando um com privilégio maior — a mesma limitação
  existe em Synergy, Barrier e no Mouse Without Borders da Microsoft.
  Rodar `target`/`tray` como Administrador resolve pra janelas elevadas
  comuns (UAC); softwares de segurança que rodam num nível ainda mais alto
  (perto de SYSTEM) podem continuar imunes de propósito. Ver
  [`docs/known-issues.md`](docs/known-issues.md).

## Estrutura

```
cmd/target/            binário que recebe conexão e injeta eventos
cmd/controller/        binário que conecta e captura eventos locais
cmd/tray/               ícone na bandeja do Windows (perfis salvos, sem terminal)
internal/protocol/     formato das mensagens de mouse/teclado na rede
internal/clipsync/      formato + conexão dedicada do clipboard compartilhado
internal/keys/          nomes de tecla independentes de SO + mapeamentos
internal/capture/       captura de input (Windows/Linux/darwin-stub)
internal/inject/        injeção de input (Windows/Linux/darwin-stub)
internal/clipboard/      leitura/escrita do clipboard do Windows (texto+imagem)
internal/tlsutil/        certificado autoassinado + trust-on-first-use
internal/auth/           checagem do token em tempo constante
docs/                    detalhes de arquitetura, bugs conhecidos e testes
scripts/build.sh          script de build multiplataforma
```

## Desenvolvimento

- [`ai-context.md`](ai-context.md) — ponto de entrada pra quem (ou qual IA)
  for mexer no projeto no dia a dia.
- [`docs/architecture.md`](docs/architecture.md) — como as peças se
  encaixam.
- [`docs/known-issues.md`](docs/known-issues.md) — bugs não óbvios já
  encontrados e por quê, pra não redescobrir do zero.
- [`docs/testing.md`](docs/testing.md) — o que tem teste automatizado, o
  que não tem (e por quê), como rodar.

## License

[MIT](LICENSE)
