# Política de segurança

## Reportar uma vulnerabilidade

**Não abra issue pública** para falhas de segurança.

- Prefira o [Private vulnerability reporting](https://github.com/backifyapp/bridge/security/advisories/new) do GitHub (aba *Security* → *Report a vulnerability*).
- Ou envie e-mail para **security@backify.app**.

Retornamos em até **72 horas** com uma avaliação inicial. Ao publicar a correção,
creditamos quem reportou (se desejar).

## Versões suportadas

Correções de segurança saem sempre na **última versão** (`latest`). Atualize com:

```sh
sudo backify-bridge update && sudo systemctl restart backify-bridge
```

| Versão | Suporte |
|---|---|
| 0.2.x | ✅ |
| 0.1.x | ❌ (atualize) |

## Modelo de segurança (resumo)

- O agent faz **apenas conexões de saída** (443/TLS). Nenhuma porta de entrada é
  aberta no servidor.
- Toda chamada à API é assinada por **HMAC-SHA256** — o segredo **nunca trafega**;
  há janela de relógio (±300s) e nonce de uso único (anti-replay).
- O segredo do agent fica em `/etc/backify-bridge/bridge.json` com permissão `0600`;
  o serviço systemd roda como usuário dedicado, sem privilégios.
- As portas expostas pelo túnel são **whitelistadas por agent** no servidor —
  um agent não alcança as portas de outro.
- O `update` valida o **SHA-256** publicado antes de trocar o binário.

### Modo Docker (opt-in)

Quando a capability **Docker** é habilitada, o agent usa o socket do Docker do
host — o que dá a ele controle sobre containers/volumes daquele servidor. Isso é
opcional e vale só para quem quer backup de volumes/containers. O helper HTTP
que faz isso escuta **apenas em `127.0.0.1`**, exige assinatura HMAC e só é
alcançável através do túnel. A config de containers (`docker inspect`) pode
conter variáveis de ambiente com segredos; elas entram no backup, sempre cifrado.
