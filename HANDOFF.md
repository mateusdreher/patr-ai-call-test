# Handoff - AI Call PoC (Go)

Este documento resume o contexto desta sessão para continuidade em outro PC.

## Objetivo do projeto

PoC em Go para pipeline de reuniões de investimento:
- ingestão (Recall webhook + upload presencial),
- transcrição,
- segmentação em tópicos,
- vínculo tópico com vídeo (clip + frame),
- entrega para serviço de análise de deals.

## Estado atual (implementado)

### 1) Ingestão
- `POST /webhooks/recall`
  - valida assinatura de webhook.
  - parseia payload Recall em formato realista (event/data/media_shortcuts).
  - normaliza e enfileira reunião para processamento.
- `POST /uploads/presencial/signed-url`
  - gera signed-url mock para upload direto.
- `POST /uploads/presencial/complete`
  - confirma upload presencial e enfileira processamento.

### 2) Processamento assíncrono
- Worker interno em memória (`chan`) simulando Pub/Sub.
- Pipeline:
  1. transcript (Recall mock ou Azure mock),
  2. tópicos (Gemini real se configurado, senão mock),
  3. assets de mídia (clip + frame mock),
  4. payload final e entrega mock.

### 3) Topic Engine
- Integração real com Gemini via API key:
  - `GEMINI_API_KEY`
  - `GEMINI_MODEL` (default: `gemini-1.5-pro`)
- Fallback automático para mock se não houver chave.

### 4) Recall realista + assinatura
- Suporte a assinatura estilo Svix:
  - `webhook-id`
  - `webhook-timestamp`
  - `webhook-signature` (`v1,<base64_hmac_sha256>`)
- Fallback legado:
  - `X-Recall-Signature` em hex.
- Endpoint utilitário:
  - `GET /demo/recall-sample`
  - retorna payload + headers + `curl` pronto.

### 5) Azure realista (mock)
- Mock no estilo Fast Transcription (`recognizedPhrases`):
  - `speaker`
  - `offsetMilliseconds`
  - `durationMilliseconds`
  - `nBest[0].display`
- Conversão para transcript unificado (`start_sec`, `end_sec`, `speaker`, `text`).

## Arquivos principais

- `cmd/demo/main.go`
  - servidor, rotas, worker, mocks, normalização e integração Gemini.
- `cmd/demo/main_test.go`
  - testes de assinatura, parse Recall, normalização Azure e pipeline.
- `README.md`
  - instruções de uso e fluxo de demo.

## Endpoints

- `GET /health`
- `POST /webhooks/recall`
- `GET /demo/recall-sample`
- `POST /uploads/presencial/signed-url`
- `POST /uploads/presencial/complete`
- `POST /demo/run`
- `GET /meetings/{meeting_id}`
- `GET /static/clips/{arquivo}`
- `GET /static/frames/{arquivo}`

## Como rodar

```bash
go test ./...
go run ./cmd/demo
```

Se o ambiente bloquear cache global do Go:

```bash
GOCACHE=/tmp/go-build-cache go test ./...
```

Para usar Gemini real:

```bash
export GEMINI_API_KEY="SUA_CHAVE"
export GEMINI_MODEL="gemini-1.5-pro"
go run ./cmd/demo
```

## Fluxos úteis de demo

### A) Pipeline rápido (sem webhook)
```bash
curl -X POST http://localhost:8080/demo/run \
  -H "Content-Type: application/json" \
  -d '{"source":"online"}'
```

### B) Webhook Recall assinado (utilitário)
```bash
curl -s http://localhost:8080/demo/recall-sample | jq -r '.curl' | sh
```

Depois consulte:
```bash
curl http://localhost:8080/meetings/<meeting_id>
```

## TODOs de produção (ainda pendentes)

- Trocar fila interna por Pub/Sub + Eventarc.
- Integrar Recall real (download autenticado de mídia/transcript conforme SDK/API).
- Integrar Azure Speech real (batch/async com diarização/biometria).
- Migrar Gemini API key para Vertex AI com IAM/ADC.
- Implementar FFmpeg real (`os/exec`) para gerar clips/frames reais.
- Entrega final real: Pub/Sub `processed-meetings` ou HTTP autenticado para equipe de deals.

## Observações

- Esta PoC prioriza demonstrar arquitetura e contratos entre etapas.
- Mocks foram deixados com comentários `PARTE 1/2/3` no código para facilitar manutenção.
