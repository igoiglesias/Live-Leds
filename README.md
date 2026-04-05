# go-router

Servidor HTTP minimalista em Go para controlar os LEDs de um roteador Linux (tipicamente OpenWrt em MIPS) através de uma interface web.

A aplicação expõe uma página em `http://<roteador>:8080` com checkboxes para cada LED disponível no sistema. Ao submeter o formulário, os LEDs selecionados são ligados (brilho `255`) e os demais são desligados (brilho `0`).

---

## Hardware alvo — TP-Link TL-WR740N

Este projeto foi feito para rodar em um **TP-Link TL-WR740N** com OpenWrt. É um roteador extremamente restrito e cada decisão de build existe em função dessas limitações.

### Especificações

| Componente | Valor |
|---|---|
| SoC | Atheros AR9331 @ 400 MHz |
| Arquitetura | MIPS 24Kc, **big-endian** |
| FPU | **ausente** (ponto flutuante em software) |
| RAM | 32 MB |
| Flash | **4 MB** |
| Rede | 2.4 GHz 802.11n (150 Mbps), 5× Fast Ethernet 100 Mbps |
| USB | não possui |

### Como cada limitação impacta o build

**Flash de 4 MB é o gargalo crítico.** Depois do firmware OpenWrt, sobram tipicamente ~500 KB a 1 MB livres — frequentemente insuficientes até para instalar `luci`. Por isso o binário é deployado em `/tmp` (tmpfs, na RAM) em vez de flash. Consequência: **o binário se perde em cada reboot** e precisa ser reenviado, ou persistido em um local alternativo.

Como cada byte conta, o Makefile combina várias técnicas de redução de tamanho:

- **`CGO_ENABLED=0`** — binário estático; sem isso o Go tentaria linkar contra a libc do roteador (uClibc/musl), introduzindo incompatibilidades e dependências extras.
- **`-ldflags="-s -w"`** — remove tabela de símbolos e informação DWARF (economiza ~25-30%).
- **`upx --lzma`** — compressão LZMA do executável (reduz mais ~60-70%).

Mesmo com tudo isso, um "hello world" em Go fica na ordem de **1 MB comprimido** — o binário `build/app-mips` deste projeto tem ~1.2 MB. Em um roteador com 32 MB de RAM total (da qual o OpenWrt já consome boa parte), carregar e descomprimir o UPX consome memória significativa na execução.

**Sem FPU → `GOMIPS=softfloat`.** O AR9331 não tem unidade de ponto flutuante. Binários compilados sem essa flag fariam instruções FP que travariam o processo (`SIGILL` / "illegal instruction"). Felizmente o Go runtime raramente precisa de FP em código típico, então o custo de performance é baixo aqui.

**Big-endian → `GOARCH=mips` (não `mipsle`).** O AR9331 é big-endian. Se o seu roteador for um chip diferente (ex: Ralink/MediaTek), verifique com `cat /proc/cpuinfo` no dispositivo — muitos modelos baratos são little-endian.

**RAM de 32 MB.** Evite estruturas grandes em memória, goroutines acumulando estado e qualquer coisa que cresça com o tempo. O GC do Go ajuda, mas o processo concorre com o OpenWrt, o dnsmasq, o hostapd e o próprio kernel. Para esse app (HTTP trivial, handlers síncronos) isso não é problema prático.

**Sem USB + flash cheia → não tente persistir dados.** Logs em arquivo, SQLite, qualquer coisa que grave em disco é mal negócio. Se precisar persistir estado, mantenha em memória ou envie para um serviço externo.

### Nomes de LED neste roteador

No TL-WR740N os LEDs em `/sys/class/leds` tipicamente aparecem como:

```
tp-link:green:lan1
tp-link:green:lan2
tp-link:green:lan3
tp-link:green:lan4
tp-link:green:qss      (WPS)
tp-link:green:system
tp-link:green:wan
tp-link:green:wlan
```

Por isso o código extrai `strings.Split(led.Name, ":")[2]` ([app.go:75](app.go#L75)) como rótulo — pega o terceiro segmento (`lan1`, `wan`, etc.). E `leds[1:]` ([app.go:72](app.go#L72)) pula o primeiro LED, que costuma ser o `system` (indicador de atividade do sistema controlado pelo kernel, que não faz sentido usuário controlar).

---

## Como funciona

O Linux expõe os LEDs do dispositivo através do sysfs em [`/sys/class/leds`](/sys/class/leds). Cada LED é um diretório contendo (entre outros) um arquivo `brightness` que pode ser lido ou escrito.

O programa:

1. Lista os diretórios em `/sys/class/leds` — cada um representa um LED físico.
2. Lê o arquivo `brightness` de cada LED para obter o estado atual.
3. Renderiza um HTML com um `<input type="checkbox">` por LED, já marcado se o brilho for maior que zero.
4. No `POST /leds/set`, escreve `255` no `brightness` dos LEDs marcados e `0` nos demais.

O estilo visual vem do [simple.css](https://simplecss.org) via CDN — não há assets locais, nenhum template e nenhuma dependência externa (apenas a stdlib do Go).

---

## Estrutura do projeto

```
go-router/
├── app.go       # Toda a lógica: tipos, handlers HTTP e HTML inline
├── go.mod       # Módulo Go (sem dependências externas)
├── Makefile     # Targets de build, cross-compile MIPS e deploy
├── build/       # Artefatos de build (gerado)
└── README.md
```

### [app.go](app.go)

- **`LED` struct** ([app.go:12-15](app.go#L12-L15)) — representa um LED (`Name`, `Brightness`).
- **`ListLEDs()`** ([app.go:19-42](app.go#L19-L42)) — percorre `/sys/class/leds`, lê o brilho atual de cada LED e retorna um slice de `LED`.
- **`SetLedBrightness()`** ([app.go:44-47](app.go#L44-L47)) — escreve o valor de brilho no arquivo sysfs do LED.
- **Handler `GET /`** ([app.go:52-93](app.go#L52-L93)) — renderiza a UI. Observação: pula o primeiro LED via `leds[1:]` ([app.go:72](app.go#L72)), provavelmente porque a primeira entrada no dispositivo-alvo corresponde a um LED de power/status que não deve ser controlável.
- **Handler `POST /leds/set`** ([app.go:95-115](app.go#L95-L115)) — processa o form, aplica o brilho e redireciona.

O roteamento usa o padrão `"METHOD path"` introduzido no `net/http` a partir do **Go 1.22** — por isso o `GO_VERSION = 1.22.10` no Makefile.

---

## Requisitos

- **Go 1.22+** (o Makefile chama `go1.22.10`; ajuste conforme sua instalação)
- **UPX** (para o target `build-mips`, que comprime o binário com LZMA)
- **SSH/SCP** configurado no roteador (para `deploy-router`)
- Acesso de escrita em `/sys/class/leds/*/brightness` — tipicamente **root**

---

## Makefile — targets disponíveis

| Target | O que faz |
|---|---|
| `make run` | Executa localmente via `go run` |
| `make build-local` | Build nativo em `build/app` |
| `make run-local` | Build nativo e executa |
| `make build-mips` | Cross-compile para MIPS (softfloat, CGO desabilitado, stripped, comprimido com UPX) |
| `make deploy-router` | Build MIPS e envia via `scp` para `/tmp` no roteador |
| `make clean` | Remove `build/` |

### Variáveis do Makefile

```make
BUILD_DIR  = build
CFLAGS     = -ldflags="-s -w"     # strip de símbolos e DWARF
ROUTER_IP  = 172.16.0.4           # IP do roteador alvo
GO_VERSION = 1.22.10
```

### Flags de cross-compile

```make
GOARCH=mips GOOS=linux GOMIPS=softfloat CGO_ENABLED=0
```

- `GOARCH=mips` — big-endian MIPS (comum em roteadores Atheros/Broadcom).
- `GOMIPS=softfloat` — emulação de ponto flutuante em software; necessário em CPUs sem FPU.
- `CGO_ENABLED=0` — binário estático, sem dependência de libc do roteador.

Se o seu roteador for MIPS **little-endian**, troque para `GOARCH=mipsle`.

---

## Uso

### Rodando localmente (para teste)

```bash
make run
```

Acesse `http://localhost:8080`. Em máquinas sem `/sys/class/leds` (macOS, por exemplo) a listagem ficará vazia.

### Deploy no roteador

1. Ajuste `ROUTER_IP` no Makefile para o IP do seu roteador.
2. Garanta acesso SSH como `root` (ou ajuste o usuário em `deploy-router`).
3. Execute:

   ```bash
   make deploy-router
   ```

4. No roteador:

   ```sh
   chmod +x /tmp/app-mips
   /tmp/app-mips &
   ```

5. Acesse `http://<ROUTER_IP>:8080` do navegador.

> Para tornar persistente, copie o binário para um local não-volátil (ex: `/usr/bin`) e crie um init script em `/etc/init.d/` (OpenWrt).

---

## Interface

A página principal exibe:

- Título "LEDs"
- Um checkbox por LED detectado, com `title` extraído do terceiro segmento do nome do LED (os nomes no sysfs geralmente seguem o padrão `<device>:<color>:<function>`, como `tp-link:green:wan`).
- Um botão **Enviar** que envia a seleção via `POST /leds/set`.

Após submissão, o servidor redireciona via `303 See Other`.

---

## Limitações conhecidas

- **Estado global**: `leds` é uma variável capturada por closure e só é repopulada no `GET /` — não há mutex. Em cargas concorrentes pode haver corrida de dados leve, mas na prática o tráfego de uma UI local de LEDs é desprezível.
- **Sem autenticação**: o servidor escuta em `:8080` sem qualquer forma de autenticação. Use apenas em redes confiáveis.
- **Brilho binário**: o controle aplica apenas `0` ou `255`; não há slider para valores intermediários.
- **Primeiro LED oculto**: o `leds[1:]` em [app.go:72](app.go#L72) pula a primeira entrada — isso é específico do dispositivo e pode precisar de ajuste em outros roteadores.
- **Redirect para `/leds`** ([app.go:114](app.go#L114)): após o POST o servidor redireciona para `/leds`, mas esta rota não está registrada — o navegador receberá `404`. Corrigir para `/` restaura o fluxo esperado.

---

## Licença

Projeto pessoal sem licença declarada.
