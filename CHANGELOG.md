# Changelog

Todas as mudanças relevantes deste projeto são documentadas aqui.
Formato baseado em [Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/);
versionamento [SemVer](https://semver.org/lang/pt-BR/).

## [Não lançado]

## [0.2.0] — 2026-07-27

### Adicionado
- **Backup e restore de Docker** (modo opt-in): volumes e containers.
  - Helper HTTP local autenticado por HMAC, alcançável só pelo túnel
    (`/docker/volumes`, `/docker/volume/{n}/export|import`,
    `/docker/container/{id}/inspect`, `/docker/container/run`, pause/unpause).
  - Export de volume via container efêmero montando o volume `:ro`.
  - Restore cria **volume/container com nome novo** por padrão (não sobrescreve
    o que está em uso).
- **Imagem oficial** `ghcr.io/backifyapp/bridge` (inclui `docker-cli`), para
  rodar o agent em container com o socket do Docker montado.
- **Inventário de frota** no heartbeat: OS, arquitetura, kernel, nº de CPUs,
  memória total, IPs e versão do Docker — exibidos no painel.
- **`backify-bridge update`**: baixa a última release, **valida o SHA-256** e
  substitui o binário.

### Alterado
- Requer Go 1.25 para compilar (dependência do cliente Chisel).

## [0.1.0] — 2026-07-26

### Adicionado
- Primeira versão do **Backify Bridge**: agent de acesso via **túnel reverso**
  (Chisel) para backup de bancos e arquivos sem abrir portas de entrada.
- `enroll` (token de uso único → credenciais locais `0600`), `run` (daemon com
  heartbeat), `status`, `version`.
- Autenticação **HMAC-SHA256** de todas as chamadas à API (o segredo nunca
  trafega), com vetor de teste travado contra o backend.
- Instalador `install.sh` + serviço `systemd` endurecido (usuário dedicado, sem
  privilégios), binários Linux amd64/arm64 no GitHub Releases.

### Correções
- Túnel reverso passa a fazer bind em `0.0.0.0` (antes `127.0.0.1`, o que
  tornava a porta inalcançável fora do container).

[0.2.0]: https://github.com/backifyapp/bridge/releases/tag/v0.2.0
[0.1.0]: https://github.com/backifyapp/bridge/releases/tag/v0.1.0
