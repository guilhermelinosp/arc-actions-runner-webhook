# arc-actions-runner-webhook

Webhook controller para criar automaticamente runners ARC quando novos repositórios são criados em `guilhermelinosp`.

## Função

Recebe webhooks do GitHub (`repository.created`) e cria `RunnerDeployment` + `HorizontalRunnerAutoscaler` no cluster para o novo repositório.

## Stack

- **Linguagem:** Go (stdlib only, zero dependências externas)
- **Imagem:** `ghcr.io/guilhermelinosp/arc-actions-runner-webhook:latest` (22MB multi-stage)
- **K8s:** kubectl embedado para criar CRDs do ARC

## Endpoints

| Método | Rota | Descrição |
|---|---|---|
| `GET /` | Health check | `runner-controller: ok` |
| `POST /` | Webhook GitHub | `repository.created` |

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `WEBHOOK_SECRET` | `""` | Secret para validar HMAC-SHA256 |
| `GITHUB_TOKEN` | `""` | Token para GitHub API |
| `NAMESPACE` | `arc-actions` | Namespace dos runners |
| `OWNER` | `guilhermelinosp` | Dono do repositório |
| `RUNNER_IMAGE` | `ghcr.io/guilhermelinosp/arc-runner:latest` | Imagem do runner |
| `PORT` | `8080` | Porta HTTP |

## Pipeline

Push na main → `ci-templates`:
1. **buildx** — build + push imagem
2. **release** — semver bump + GitHub Release
3. **push** — tag image com versão

## Deploy

Gerenciado via ArgoCD em `arc-actions`:
```
github.com/guilhermelinosp/arc-actions/webhook/deployment.yaml
```
