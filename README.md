# arc-actions-runner-webhook

Webhook controller para criar automaticamente runners ARC quando novos repositórios são criados em `guilhermelinosp`.

## Stack

- **Linguagem:** Go + `client-go` (K8s dynamic client)
- **Imagem:** `ghcr.io/guilhermelinosp/arc-actions-runner-webhook:latest` (13MB distroless/static)
- **K8s:** dynamic client via `k8s.io/client-go` (sem kubectl)
- **Logs:** JSON estruturado via `log/slog`
- **Métricas:** Prometheus em `:9090/metrics`

## Endpoints

| Porta | Rota | Descrição |
|---|---|---|
| `8080` | `GET /` | Health check → `runner-controller: ok` |
| `8080` | `POST /` | Webhook GitHub `repository.created` |
| `9090` | `GET /metrics` | Prometheus metrics |

## Métricas

| Nome | Tipo | Labels |
|---|---|---|
| `webhook_events_total` | Counter | `event`, `status` |
| `runner_creation_duration_seconds` | Histogram | — |
| `runners_active_total` | Gauge | — |

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `WEBHOOK_SECRET` | `""` | Secret para validar HMAC-SHA256 |
| `GITHUB_TOKEN` | `""` | Token para GitHub API (workflows check) |
| `NAMESPACE` | `arc-actions` | Namespace dos runners |
| `OWNER` | `guilhermelinosp` | Dono do repositório |
| `RUNNER_IMAGE` | `ghcr.io/guilhermelinosp/arc-runner:latest` | Imagem do runner |
| `PORT` | `8080` | Porta HTTP |

## Pipeline

Push na main → `ci-templates`:
1. **buildx** — build + push imagem (multi-arch amd64)
2. **release** — semver bump + GitHub Release
3. **push** — tag image com versão

## Deploy

Gerenciado via ArgoCD em `arc-actions`:
```
github.com/guilhermelinosp/arc-actions/webhook/deployment.yaml
```
