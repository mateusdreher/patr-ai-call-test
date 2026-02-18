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
- `GET /demo/recall-sample` (gera payload + headers assinados + curl pronto)
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

Exemplo de payload realista (estilo Recall webhook):

```json
{
  "event": "recording.done",
  "data": {
    "bot": {
      "id": "bot_123",
      "metadata": { "meeting_id": "mtg-recall-001" }
    },
    "recording": { "id": "rec_123", "duration": 3600 },
    "media_shortcuts": {
      "video_mixed": { "data": { "download_url": "https://recall.example/video.mp4" } },
      "transcript": { "data": { "download_url": "https://recall.example/transcript.json" } }
    }
  }
}
```

Headers esperados (estilo Svix):
- `webhook-id`
- `webhook-timestamp`
- `webhook-signature` no formato `v1,<base64_hmac_sha256>`

Obs.: o código mantém fallback legado com `X-Recall-Signature` para compatibilidade com demos antigas.

### Endpoint utilitário para webhook Recall

Use:

```bash
curl http://localhost:8080/demo/recall-sample
```

A resposta traz:
- payload realista do webhook;
- headers assinados no padrão Svix;
- comando `curl` pronto para enviar no `/webhooks/recall`.

Exemplo direto (1-linha):

```bash
curl -s http://localhost:8080/demo/recall-sample | jq -r '.curl' | sh
```

Depois consulte:

```bash
curl http://localhost:8080/meetings/<meeting_id>
```

## Mock Azure (Fast Transcription)

O mock presencial usa estrutura inspirada em `recognizedPhrases` do Fast Transcription:
- `speaker`
- `offsetMilliseconds`
- `durationMilliseconds`
- `nBest[0].display`

Esses campos são normalizados para o formato interno unificado (`start_sec`, `end_sec`, `speaker`, `text`).

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
