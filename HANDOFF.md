# Handoff - Recall Review PoC

## Estado atual

O projeto agora tem duas frentes no mesmo servidor:

1. Pipeline original de reunioes (`/webhooks/recall`, `/demo/run`, `/meetings/{id}`).
2. Camada de review da Recall API (`/api/review/recall/*`) + UI em `/review/`.

## O que foi adicionado

- Novo servico em [cmd/demo/recall_review.go](/home/mateusdreher/.projects/zappts/patr-ai-call-test/cmd/demo/recall_review.go)
- Novas rotas registradas em [cmd/demo/main.go](/home/mateusdreher/.projects/zappts/patr-ai-call-test/cmd/demo/main.go)
- UI estatica em:
  - [web/index.html](/home/mateusdreher/.projects/zappts/patr-ai-call-test/web/index.html)
  - [web/styles.css](/home/mateusdreher/.projects/zappts/patr-ai-call-test/web/styles.css)
  - [web/app.js](/home/mateusdreher/.projects/zappts/patr-ai-call-test/web/app.js)
- Testes adicionais em [cmd/demo/main_test.go](/home/mateusdreher/.projects/zappts/patr-ai-call-test/cmd/demo/main_test.go)

## Endpoints novos relevantes

- `GET /api/review/recall/catalog`
- `GET /api/review/recall/state`
- `POST /api/review/recall/reset`
- `POST /api/review/recall/bootstrap`
- `GET /api/review/recall/webhooks/samples`
- `GET|POST /api/review/recall/bots`
- `GET|PATCH|DELETE /api/review/recall/bots/{bot_id}`
- `POST /api/review/recall/bots/{bot_id}/start_recording`
- `POST /api/review/recall/bots/{bot_id}/pause_recording`
- `POST /api/review/recall/bots/{bot_id}/resume_recording`
- `POST /api/review/recall/bots/{bot_id}/stop_recording`
- `POST /api/review/recall/bots/{bot_id}/leave`
- `POST|DELETE /api/review/recall/bots/{bot_id}/output_media`
- `GET /api/review/recall/recordings`
- `GET|DELETE /api/review/recall/recordings/{recording_id}`
- `GET /api/review/recall/transcripts/{id}`
- `GET /api/review/recall/video_mixed/{id}`
- `GET /api/review/recall/audio_mixed/{id}`
- `GET /api/review/recall/meeting_metadata/{id}`
- `GET /api/review/recall/participant_events/{id}`
- `GET|POST /api/review/recall/calendar/meetings`
- `GET|PUT /api/review/recall/calendar/meetings/{meeting_id}`
- `POST /api/review/recall/calendar/meetings/refresh`

## Como validar localmente

```bash
GOCACHE=/tmp/go-build-cache go test ./...
go run ./cmd/demo
```

Depois abrir:

```text
http://localhost:8080/review/
```

## Referencias oficiais usadas

- Getting Started: https://docs.recall.ai/docs/getting-started?utm_source=Recall.ai&utm_medium=navbar&utm_content=API_Docs
- Bot Overview: https://docs.recall.ai/docs/bot-overview
- Media Shortcuts: https://docs.recall.ai/docs/media-shortcuts
- Async Transcription: https://docs.recall.ai/docs/async-transcription
- Output Media: https://docs.recall.ai/v1.10/docs/stream-media
- List Bots reference: https://docs.recall.ai/v1.10/reference/bot_list

## Pontos de atencao

- A camada `/api/review/recall/*` e mockada para review; nao chama a Recall real.
- Os links de referencia misturam docs gerais e algumas paginas `v1.10` encontradas na documentacao publica.
- Os artefatos de media gerados no review sao escritos em `DATA_DIR/review` e servidos por `/static/review/...`.
