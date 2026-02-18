# AI Call - Demo de Arquitetura (Go)

PoC em Go para demonstrar seu fluxo:

1. Ingestão (Recall webhook ou upload presencial com signed URL).
2. Processamento assíncrono (mock de Pub/Sub + worker).
3. Transcrição unificada (Recall/Azure mockados).
4. Segmentação em tópicos (Vertex/Gemini mockado).
5. Vínculo fala + vídeo (clips e "captura de tela" mockados por tópico).
6. Montagem de payload final e entrega para time de deals (mockado).

## Executar

```bash
go test ./...
go run ./cmd/demo
```

Servidor padrão: `http://localhost:8080`

## Usar Gemini Pro real (sem mock)

Defina sua chave antes de subir:

```bash
export GEMINI_API_KEY="SUA_CHAVE"
export GEMINI_MODEL="gemini-1.5-pro"
go run ./cmd/demo
```

Se `GEMINI_API_KEY` estiver vazio, o sistema usa segmentação mockada automaticamente.

## Endpoints principais

- `GET /health`
- `POST /webhooks/recall`
- `POST /uploads/presencial/signed-url`
- `POST /uploads/presencial/complete`
- `POST /demo/run` (atalho para gerar reunião mockada)
- `GET /meetings/{meeting_id}`
- `GET /static/clips/{arquivo}`
- `GET /static/frames/{arquivo}`

## Fluxo rápido para demo

1. Disparar uma reunião mockada online:

```bash
curl -X POST http://localhost:8080/demo/run \
  -H "Content-Type: application/json" \
  -d '{"source":"online"}'
```

2. Consultar resultado:

```bash
curl http://localhost:8080/meetings/<meeting_id>
```

No JSON final você verá cada tópico com:
- `start_sec` / `end_sec`
- `clip_url`
- `frame_url`
- `frame_notes` (explica o vínculo do momento de tela com o que foi falado)

## Simulação de webhook Recall com HMAC

Secret padrão: `demo-secret` (`RECALL_WEBHOOK_SECRET`).

Exemplo de payload:

```json
{
  "meeting_id": "mtg-recall-001",
  "mp4_url": "https://recall.example/video.mp4",
  "transcript_url": "https://recall.example/transcript.json",
  "duration_seconds": 3600
}
```

O header esperado é `X-Recall-Signature` com HMAC-SHA256 em hex do body.

## TODOs para produção (já marcados no código)

- Recall.ai real:
  - validar formato exato de assinatura/header conforme doc oficial.
  - baixar MP4/transcript real com autenticação.
- Pub/Sub/Eventarc:
  - trocar `chan` por tópico/subscription real.
- Azure Speech:
  - integrar SDK/API para diarização/biometria com perfis de voz.
- Vertex AI Gemini:
  - hoje a PoC usa Gemini via API key para facilitar demo local.
  - migrar para Vertex AI com autenticação GCP (ADC/IAM) e controles de projeto/região.
- Video Factory:
  - usar `os/exec` + `ffmpeg` para cortes reais e captura de frames reais.
- Delivery final:
  - publicar em tópico `processed-meetings` ou fazer POST autenticado para o outro time.
