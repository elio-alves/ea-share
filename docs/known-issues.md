# Bugs conhecidos e por quê

Cada item aqui foi um bug real, diagnosticado e corrigido (ou documentado
como limitação aceita). Objetivo: não redescobrir a mesma causa raiz do
zero da próxima vez que o sintoma aparecer parecido.

## Deltas de mouse via `pt`-diffing do hook quebram quando suprimido

**Sintoma**: logo após "engaged", o log mostrava "disengaged" quase no
mesmo instante — um round-trip espúrio.

**Causa**: `internal/capture/capture_windows.go` calculava o delta do
mouse comparando a posição absoluta (`pt`) entre chamadas consecutivas do
hook `WH_MOUSE_LL`. Assim que o `controller` começa a suprimir o input
local (engaged), o Windows **não confirma** a nova posição do cursor —
então `pt` para de acumular de forma confiável, e a diferença entre
eventos vira ruído. Esse ruído às vezes batia com a condição de "empurrou
de volta pela borda de entrada" segundos depois de engajar.

**Fix**: captura de delta migrada pra API **Raw Input** do Windows
(`RegisterRawInputDevices` + `WM_INPUT` numa janela oculta, lendo
`RAWMOUSE.lLastX/lLastY`) — dá o delta relativo de verdade, HID puro,
independente de supressão/clamping do cursor. `WH_MOUSE_LL` hoje só cuida
de detectar a borda (precisa de posição absoluta) e suprimir/repassar
clique/scroll/teclado. Mesma técnica que Synergy/Barrier usam no Windows.

**Lição**: qualquer coisa que faça diff de posição do cursor do Windows
através de um trecho onde o input está sendo suprimido ou injetado não é
confiável — prefira deltas de Raw Input ou um warp explícito de
recentralização.

## `BI_BITFIELDS` não suportado ao ler imagem do clipboard

**Sintoma**: colar um print de tela (`PrintScreen`) falhava silenciosamente
— nada chegava do outro lado, e o log do controller mostrava
`clipboard: unsupported DIB compression 3`.

**Causa**: `internal/clipboard.ReadImagePNG` só sabia interpretar
`BI_RGB` (compressão 0). Só que `BI_BITFIELDS` (compressão 3) é o formato
que o Windows **quase sempre** usa pra bitmap de 32bpp de captura de tela
— as 3 máscaras de canal de cor (R/G/B) vêm em 3 DWORDs extras logo após o
`BITMAPINFOHEADER`, em vez de posição de byte fixa.

**Fix**: aceitar `BI_BITFIELDS` pra 32bpp, ler as 3 máscaras e extrair
cada canal via `bits.TrailingZeros32(mask)` como shift, em vez de assumir
byte fixo.

**Lição**: não assuma o caso mais simples (`BI_RGB`) ao ler dado real de
bitmap do Windows — `BI_BITFIELDS` é o caso comum pra 32bpp, não o raro.

## `Ctrl+Alt+V` colava o combo errado no app focado

**Sintoma**: o clipboard chegava certinho no outro lado (log mostrava
"received text/image, pasting"), mas nada aparecia no app.

**Causa**: o atalho `Ctrl+Alt+V` só suprime a tecla `V` — o `Ctrl` e o
`Alt` continuam sendo encaminhados/injetados normalmente como teclas
comuns. No momento de colar, o `Ctrl+V` sintético era injetado **em cima**
de um `Alt` que o app ainda via como pressionado — ou seja, o app recebia
`Ctrl+Alt+V` de verdade, que não é atalho de colar em lugar nenhum.

**Fix**: `injectPaste` (em `cmd/target/clipboard_windows.go` e
`cmd/controller/clipboard_windows.go`) agora solta explicitamente
`ControlLeft/Right`, `AltLeft/Right` e `ShiftLeft/Right` **antes** de
injetar o `Ctrl+V` limpo.

## Tecla modificadora "presa" pelo autorepeat do `V`

**Sintoma**: depois de usar o clipboard, o teclado do controller "bugava"
— digitar normal disparava atalhos do Windows (abrir janelas etc.), e nem
matar o app resolvia.

**Causa**: `V` (diferente de `Ctrl`/`Alt`, que não repetem) tem autorepeat
do Windows enquanto fica pressionado. O handler do atalho disparava a
**cada repetição**, criando goroutines de `injectPaste` sobrepostas que
competiam entre si soltando/pressionando `Ctrl` — deixando um modificador
"preso" no nível do SO.

**Fix**: só dispara uma vez por pressão física (guarda em
`!s.hotkeyVDown` em `internal/capture/capture_windows.go`) + mutex
serializando `injectPaste` nos dois lados, como reforço.

**Truque de recuperação ao vivo** (sem precisar reiniciar o Windows): um
`Ctrl+Alt+Del` físico reseta o estado — é tratado pela Secure Attention
Sequence do Windows, abaixo de qualquer hook de modo usuário, e a troca de
desktop limpa o estado de tecla presa como efeito colateral. Também vale
tentar: apertar e soltar sozinho o modificador preso (hardware de
verdade), o Teclado Virtual, ou fazer logoff/login.

## Ícones de bandeja de processos elevados não respondem ao input injetado

**Sintoma**: passar o cursor (controlado remotamente) sobre certos ícones
da bandeja do Windows (ex. antivírus) trava o controle na hora; mexer o
mouse físico de verdade na máquina alvo destrava.

**Causa**: **UIPI** (User Interface Privilege Isolation), uma proteção do
próprio Windows — input sintético (`SendInput`, como o `target` injeta o
cursor) de um processo com privilégio menor é bloqueado de afetar UI de um
processo com privilégio maior. Confirmado rodando `target`/`tray` como
Administrador: ícones "normais" pararam de travar, mas o do antivírus
continuou — o componente de bandeja dele provavelmente roda num nível de
integridade ainda mais alto (perto de SYSTEM), de propósito, como
proteção anti-adulteração.

**Não é bug nosso.** Confirmado que Synergy, Barrier e o Mouse Without
Borders da própria Microsoft têm exatamente a mesma limitação com janelas
elevadas/UAC — é uma fronteira de segurança do Windows, não uma falha de
implementação. Rodar como Administrador ajuda com elevação comum (UAC);
não tem como (nem faria sentido) subir pra SYSTEM só por causa disso.

## `tray.log` não captura crash do próprio `tray.exe`

**Status: identificado, ainda não corrigido.**

`tray.log` só captura a saída dos processos filhos (`target`/`controller`)
via `log.SetOutput` do pacote `log`. Um panic/crash do próprio `tray.exe`
(que roda sem console, `-H=windowsgui`) não vai pra lugar nenhum — a
última linha do log antes dele sumir é sempre de um processo filho, nunca
do próprio tray. Aconteceu de verdade numa suspensão/hibernação de
madrugada: o processo `target` filho saiu com erro, o tray bateu um erro
interno de tooltip do systray no mesmo segundo, e depois nada — o processo
tinha sumido.

**Mitigação proposta** (não implementada): redirecionar os handles reais
de stdout/stderr do processo (não só `log.SetOutput`, que só cobre a
biblioteca `log`) pro arquivo de log logo no início do `main()`, mais um
limite de tamanho/rotação.
