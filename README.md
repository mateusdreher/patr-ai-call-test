# AI Call - Recall Review PoC (Go)

PoC em Go para duas frentes:

1. Pipeline original de ingestao/processamento de reunioes.
2. Camada de review da Recall API com UI simples, endpoints mockados e cobertura dos fluxos principais de bots, recordings, media shortcuts, output media, webhooks e calendar.

## Rodar

```bash
GOCACHE=/tmp/go-build-cache go test ./...
go run ./cmd/demo
```

Servidor padrao: `http://localhost:8080`

Entradas principais:

- Review UI: `http://localhost:8080/review/`
- Health: `http://localhost:8080/health`
- Pipeline demo: `http://localhost:8080/demo/run`
- Documento da apresentacao: `docs/recall-poc-apresentacao.md`

## O que esta POC cobre da Recall

Com base na documentacao oficial da Recall, a POC agora demonstra localmente:

- criacao, listagem, consulta, edicao e remocao de bots;
- start, pause, resume, stop de gravacao e leave call;
- recordings com `media_shortcuts`;
- acesso a `transcript`, `video_mixed`, `audio_mixed`, `meeting_metadata` e `participant_events`;
- configuracao de `output_media` para cenarios de AI agents;
- reunioes de calendario para auto-join;
- exemplos de webhooks como `bot.joined`, `recording.started`, `recording.done` e `bot.left`.
- agenda dedicada / fluxo de calendar scheduling para auto-join;
- identidade de participantes com destaque para a limitacao de email derivado via calendar match;
- pontos de governanca como cancelamento de bot agendado, leave call e delecao de media.

Referencias oficiais usadas para estruturar essa camada:

- Getting Started: https://docs.recall.ai/docs/getting-started?utm_source=Recall.ai&utm_medium=navbar&utm_content=API_Docs
- Bot Overview: https://docs.recall.ai/docs/bot-overview
- Media Shortcuts: https://docs.recall.ai/docs/media-shortcuts
- Asynchronous Transcription: https://docs.recall.ai/docs/async-transcription
- Output Media / Send AI Agents to Meetings: https://docs.recall.ai/v1.10/docs/stream-media
- List Bots reference: https://docs.recall.ai/v1.10/reference/bot_list

Obs.: a UI local e os endpoints `/api/review/recall/*` sao um espelho didatico para review. Eles nao substituem a API oficial nem garantem paridade perfeita de schema/versionamento.

## Front de review

O front simples em `HTML + CSS + JS` esta em `web/` e consome:

- `GET /api/review/recall/catalog`
- `GET /api/review/recall/state`
- acoes de bot/recording/calendar em `/api/review/recall/...`

Na tela de review voce consegue:

- ver o catalogo de endpoints com link para a doc oficial;
- ver os endpoints classificados por prioridade para a POC;
- criar bot e calendar meeting pela UI;
- executar `start/pause/resume/stop/leave` em bots;
- inspecionar recordings e media shortcuts gerados;
- validar metadata de participantes com nomes, emails derivados e casos `null`;
- abrir exemplos de payload de webhook;
- apresentar o mapeamento entre endpoint oficial e endpoint local da demo.

## Endpoints prioritarios para esta POC

Fluxo recomendado para validar o caso de uso:

1. `POST /api/review/recall/calendar/meetings`
2. `POST /api/review/recall/bots`
3. `POST /api/review/recall/bots/{bot_id}/start_recording`
4. `POST /api/review/recall/bots/{bot_id}/stop_recording`
5. `GET /api/review/recall/recordings/{recording_id}`
6. `GET /api/review/recall/transcripts/{transcript_id}`
7. `GET /api/review/recall/meeting_metadata/{metadata_id}`
8. `GET /api/review/recall/participant_events/{event_id}`
9. `DELETE /api/review/recall/recordings/{recording_id}`

Resumo conceitual da POC em [docs/recall-poc-apresentacao.md](docs/recall-poc-apresentacao.md).

## Endpoints do pipeline original

### `GET /health`

Healthcheck simples do servidor.

### `POST /webhooks/recall`

Recebe webhook Recall assinado, valida assinatura, normaliza o payload e enfileira a reuniao para o pipeline interno.

### `GET /demo/recall-sample`

Gera payload e headers assinados para simular um webhook Recall no padrao Svix.

### `POST /uploads/presencial/signed-url`

Cria uma signed URL mock para upload do video presencial.

### `POST /uploads/presencial/complete`

Confirma o upload presencial e dispara processamento.

### `POST /demo/run`

Atalho para gerar uma reuniao mockada online ou presencial.

### `GET /meetings/{meeting_id}`

Retorna o estado consolidado da reuniao processada.

### `GET /static/clips/{arquivo}`

Serve clips mockados gerados por topico.

### `GET /static/frames/{arquivo}`

Serve frames mockados gerados por topico.

## Endpoints da camada Recall Review

### Catalogo e estado

#### `GET /api/review/recall/catalog`

Lista o catalogo local de endpoints Recall contemplados pela review. Cada item informa:

- categoria;
- descricao funcional;
- endpoint oficial;
- endpoint local correspondente;
- link da doc oficial;
- como o endpoint aparece na apresentacao.

#### `GET /api/review/recall/state`

Retorna o estado agregado da demo:

- resumo com contadores;
- bots;
- recordings;
- calendar meetings;
- webhook samples;
- link de entrada da documentacao oficial.

#### `POST /api/review/recall/bootstrap`

Garante carga inicial para demo, incluindo:

- 1 bot com recording concluido e media shortcuts;
- 1 bot agendado com `output_media`;
- 1 calendar meeting ligado ao bot agendado.

#### `POST /api/review/recall/reset`

Reseta o estado mockado e recria a massa padrao da review.

### Webhooks

#### `GET /api/review/recall/webhooks/samples`

Explica e retorna payloads de exemplo para:

- `bot.joined`
- `recording.started`
- `recording.done`
- `bot.left`

### Bots

#### `GET /api/review/recall/bots`

Lista todos os bots mockados da demo.

#### `POST /api/review/recall/bots`

Cria um bot mockado para review. Aceita os principais campos de criacao:

- `meeting_url`
- `join_at`
- `bot_name`
- `recording_mode`
- `transcription`
- `metadata`
- `output_media`
- `real_time_features`
- `config`

#### `GET /api/review/recall/bots/{bot_id}`

Retorna o detalhe de um bot especifico.

#### `PATCH /api/review/recall/bots/{bot_id}`

Atualiza campos mutaveis do bot:

- `status`
- `bot_name`
- `meeting_url`
- `metadata`
- `join_at`
- `config_patch`

#### `DELETE /api/review/recall/bots/{bot_id}`

Remove o bot da demo e apaga recordings associados.

#### `POST /api/review/recall/bots/{bot_id}/start_recording`

Abre uma recording `in_progress` para o bot.

#### `POST /api/review/recall/bots/{bot_id}/pause_recording`

Muda a gravacao ativa para `paused`.

#### `POST /api/review/recall/bots/{bot_id}/resume_recording`

Retoma a gravacao pausada.

#### `POST /api/review/recall/bots/{bot_id}/stop_recording`

Finaliza a gravacao ativa e gera:

- `video_mixed`
- `audio_mixed`
- `transcript`
- `meeting_metadata`
- `participant_events`

Esses artefatos aparecem em `media_shortcuts` e apontam para arquivos servidos localmente em `/static/review/`.

#### `POST /api/review/recall/bots/{bot_id}/leave`

Simula o bot saindo da chamada. Se existir gravacao ativa, ela e concluida.

#### `POST /api/review/recall/bots/{bot_id}/output_media`

Atualiza a configuracao de `output_media` do bot para review de AI agents.

#### `DELETE /api/review/recall/bots/{bot_id}/output_media`

Remove `output_media` do bot.

### Recordings

#### `GET /api/review/recall/recordings`

Lista todas as recordings mockadas.

#### `GET /api/review/recall/recordings/{recording_id}`

Retorna uma recording especifica com `media_shortcuts`.

#### `DELETE /api/review/recall/recordings/{recording_id}`

Remove a recording da demo.

### Media objects

#### `GET /api/review/recall/transcripts/{transcript_id}`

Retorna o media object de transcript com `download_url`.

#### `GET /api/review/recall/video_mixed/{video_id}`

Retorna o media object do MP4 consolidado.

#### `GET /api/review/recall/audio_mixed/{audio_id}`

Retorna o media object do audio consolidado.

#### `GET /api/review/recall/meeting_metadata/{metadata_id}`

Retorna o media object com metadata estrutural da reuniao.

#### `GET /api/review/recall/participant_events/{event_id}`

Retorna o media object com timeline de participacao.

### Calendar

#### `GET /api/review/recall/calendar/meetings`

Lista reunioes mockadas oriundas de calendario.

#### `POST /api/review/recall/calendar/meetings`

Cria uma reuniao mockada de calendario com:

- `platform`
- `meeting_url`
- `starts_at`
- `calendar_user`
- `metadata`

#### `GET /api/review/recall/calendar/meetings/{meeting_id}`

Consulta uma reuniao especifica de calendario.

#### `PUT /api/review/recall/calendar/meetings/{meeting_id}`

Atualiza dados de uma reuniao de calendario.

#### `POST /api/review/recall/calendar/meetings/refresh`

Simula um refresh/sync do calendario para revisar o fluxo pre-call.

## Fluxo rapido para review

1. Suba o servidor:

```bash
go run ./cmd/demo
```

2. Abra a UI:

```text
http://localhost:8080/review/
```

3. Na review, percorra:

- `Bootstrap demo`
- `Criar bot`
- `Start recording`
- `Pause`
- `Resume`
- `Stop`
- abrir `transcript` e `video_mixed`
- criar `calendar meeting`

4. Se quiser validar por API, consulte:

```bash
curl http://localhost:8080/api/review/recall/catalog | jq
curl http://localhost:8080/api/review/recall/state | jq
```

## Fluxo rapido do pipeline original

### Reuniao mockada online

```bash
curl -X POST http://localhost:8080/demo/run \
  -H "Content-Type: application/json" \
  -d '{"source":"online"}'
```

### Consultar resultado

```bash
curl http://localhost:8080/meetings/<meeting_id>
```

### Simular webhook Recall assinado

```bash
curl -s http://localhost:8080/demo/recall-sample | jq -r '.curl' | sh
```

## Gemini real

Se quiser usar Gemini para segmentacao em vez de mock:

```bash
export GEMINI_API_KEY="SUA_CHAVE"
export GEMINI_MODEL="gemini-1.5-pro"
go run ./cmd/demo
```

Sem `GEMINI_API_KEY`, a segmentacao continua mockada.

## Testes

```bash
GOCACHE=/tmp/go-build-cache go test ./...
```

Cobertura principal validada:

- assinatura de webhook Recall;
- parse do webhook Recall;
- normalizacao Azure mock;
- pipeline online;
- ciclo de vida bot -> start recording -> stop recording -> media shortcuts;
- exposicao do catalogo Recall.

## TODOs de producao

- substituir mocks `/api/review/recall/*` por clientes reais da Recall se o objetivo passar de review para integracao;
- alinhar cada schema local com a versao exata da API oficial escolhida;
- trocar fila em memoria por Pub/Sub/Eventarc;
- integrar download autenticado de midia/transcript da Recall;
- integrar Azure Speech real;
- migrar Gemini para Vertex AI com IAM/ADC;
- usar FFmpeg real para clips e frames;
- entregar payload final real para o time de deals.
