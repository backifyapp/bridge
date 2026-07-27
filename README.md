# Backify Bridge

**Broker de acesso seguro para backups do [Backify](https://backify.app).**

O Bridge é um agent leve que roda no seu servidor Linux e dá ao Backify acesso
seguro aos seus serviços locais (banco de dados, arquivos) **sem você abrir
nenhuma porta de entrada no firewall e sem entregar suas credenciais de produção
para fora**.

Ele disca **para fora** (443/TLS), mantém um túnel reverso e expõe **apenas** os
serviços que você autorizou no painel. O Bridge **não faz backup**: quem processa
é o worker do Backify, que alcança o seu banco/arquivos *através* do túnel. Por
isso o Bridge é minúsculo e o seu servidor **não precisa de mais nada** — nem
Docker, nem `pg_dump`, nem `restic`.

```
  Seu servidor                              Backify
 ┌────────────────────┐   saída 443/TLS   ┌───────────────────────┐
 │ backify-bridge     │ ────────────────► │ túnel (rede interna)  │
 │  expõe localhost:  │   túnel reverso   │        │              │
 │   5432 (postgres)  │ ◄──────────────── │        ▼              │
 │   22   (arquivos)  │                   │   worker → pg_dump    │
 └────────────────────┘                   └───────────────────────┘
     ▲ nada de porta de entrada aberta
```

## Por que usar

- **Firewall fechado.** Só conexão de saída; nada exposto à internet.
- **Credenciais ficam com você.** O worker acessa via túnel; nada de expor o banco.
- **Bancos `localhost-only`.** Faz backup de bancos que só escutam em `127.0.0.1`.
- **Zero dependências.** Um binário estático. Sem Docker, sem clientes de banco.

## Instalação

Linux **amd64/arm64**, roda como serviço `systemd`.

```sh
curl -fsSL https://raw.githubusercontent.com/backifyapp/bridge/main/install.sh | sudo sh -s -- --token <SEU_TOKEN>
```

O `<SEU_TOKEN>` (enrollment token) é gerado no painel do Backify ao criar um
Bridge. Ou manualmente:

```sh
sudo -u backify-bridge backify-bridge enroll --token <SEU_TOKEN>
sudo systemctl enable --now backify-bridge
backify-bridge status
```

## Como funciona

1. **Enroll** — o Bridge troca o enrollment token de uso único por uma identidade
   de máquina + um segredo HMAC, guardados em `/etc/backify-bridge/bridge.json`
   (permissão `0600`).
2. **Heartbeat** — periodicamente reporta que está vivo e recebe do Backify
   **quais serviços expor** (a regra fica no Backify, não no agent).
3. **Túnel** — mantém a conexão de saída de pé e expõe só as portas autorizadas.

Todas as chamadas à API (menos o enroll) são assinadas por **HMAC-SHA256** — o
segredo **nunca trafega**, só assina. Veja [`internal/sign`](internal/sign).

## Modo Docker (volumes e containers)

Além de bancos/arquivos, o Bridge faz backup/restore de **volumes** e
**containers** Docker (capability opt-in `Docker` no painel). O agent sobe um
helper HTTP local (só pelo túnel, HMAC) que exporta o volume como `tar.gz` (via
container efêmero `:ro`) e lê a config do container (`docker inspect`); o restic
roda no worker. Restore recria volume/container com **nome novo** por padrão.

Precisa do socket do Docker — use a imagem oficial:

```sh
docker run -d --name backify-bridge \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v /etc/backify-bridge:/etc/backify-bridge \
  backifyapp/bridge
```

> ⚠️ Acesso ao socket do Docker é **root-equivalente** no host. `docker inspect`
> pode expor segredos (env vars) — vão para o snapshot cifrado. Habilite só onde
> você confia no Backify. Banco em container: prefira a origem de banco pelo túnel.

## Transporte

O transporte do túnel fica atrás da interface [`transport.Transport`](internal/transport/transport.go).
A implementação v1 é **Chisel** ([`chisel.go`](internal/transport/chisel.go); TCP
sobre HTTPS, cripto SSH), embutida como client — o servidor só roda o binário do
Bridge. A interface existe de propósito: migrar para **FRP** no futuro é uma nova
implementação, sem mexer no daemon.

O agent autentica no chisel-server com a própria identidade (`agentID:secret`),
validada lá por um plugin contra a API do Backify. Cada serviço vira um túnel
reverso `R:<bind no server>:localhost:<porta local>` — o *bind* é atribuído pelo
control plane e informado no heartbeat.

> **Status (Fase 1):** o client Chisel está implementado. Ele espera o Backify
> provisionar o túnel — enquanto o heartbeat não trouxer `tunnel.server` + a
> porta remota de cada serviço (Fase 2: `chisel-server` + alocação de portas no
> control plane), o transporte fica ocioso sem erro. Para exercitar o fluxo sem
> servidor, use `BACKIFY_BRIDGE_STUB=1`. Rode `go mod tidy` após clonar.

## Segurança

- Só saída no cliente; nenhuma porta de entrada aberta.
- Segredo HMAC em repouso com `0600`; identidade por-agent, revogável no painel.
- Endpoints do túnel são privados (só o worker alcança), nunca públicos.
- O serviço systemd roda como usuário dedicado, sem privilégios (hardening).

## Desenvolvimento

```sh
go test ./...      # inclui o vetor que prova a compatibilidade HMAC com o backend
go build ./...
go run ./cmd/backify-bridge status

# Teste de aceitação do Docker (roundtrip real de volume) — num host com docker:
go test -tags e2e ./internal/docker
```

Config de dev em outro caminho: `BACKIFY_BRIDGE_CONFIG=./bridge.json`.

## Licença

[MIT](LICENSE).
