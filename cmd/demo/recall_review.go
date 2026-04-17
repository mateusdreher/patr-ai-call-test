package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type RecallReviewService struct {
	cfg              Config
	mu               sync.RWMutex
	bots             map[string]*RecallReviewBot
	recordings       map[string]*RecallReviewRecording
	calendarMeetings map[string]*RecallCalendarMeeting
	webhookSamples   []RecallWebhookSample
	catalog          []RecallEndpointDoc
}

type RecallReviewSummary struct {
	Bots             int `json:"bots"`
	Recordings       int `json:"recordings"`
	CalendarMeetings int `json:"calendar_meetings"`
	Completed        int `json:"completed_recordings"`
	InProgress       int `json:"in_progress_recordings"`
}

type RecallReviewState struct {
	GeneratedAt       time.Time               `json:"generated_at"`
	Summary           RecallReviewSummary     `json:"summary"`
	Bots              []RecallReviewBot       `json:"bots"`
	Recordings        []RecallReviewRecording `json:"recordings"`
	CalendarMeetings  []RecallCalendarMeeting `json:"calendar_meetings"`
	WebhookSamples    []RecallWebhookSample   `json:"webhook_samples"`
	ReviewHints       []string                `json:"review_hints"`
	OfficialDocsEntry string                  `json:"official_docs_entry"`
}

type RecallEndpointDoc struct {
	Category     string `json:"category"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Official     string `json:"official"`
	Local        string `json:"local"`
	DocsURL      string `json:"docs_url"`
	Resource     string `json:"resource"`
	Presentation string `json:"presentation"`
	Priority     string `json:"priority"`
}

type RecallReviewBot struct {
	ID               string            `json:"id"`
	Status           string            `json:"status"`
	MeetingURL       string            `json:"meeting_url,omitempty"`
	JoinAt           *time.Time        `json:"join_at,omitempty"`
	RecordingMode    string            `json:"recording_mode"`
	Transcription    string            `json:"transcription"`
	BotName          string            `json:"bot_name"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	OutputMedia      map[string]any    `json:"output_media,omitempty"`
	RecordingIDs     []string          `json:"recording_ids"`
	RealTimeFeatures []string          `json:"real_time_features,omitempty"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Links            map[string]string `json:"links"`
	Config           map[string]any    `json:"config,omitempty"`
}

type RecallReviewRecording struct {
	ID             string                       `json:"id"`
	BotID          string                       `json:"bot_id"`
	Status         string                       `json:"status"`
	StartedAt      time.Time                    `json:"started_at"`
	CompletedAt    *time.Time                   `json:"completed_at,omitempty"`
	DurationSec    int                          `json:"duration_sec"`
	MediaShortcuts map[string]RecallMediaObject `json:"media_shortcuts"`
	Artifacts      map[string]string            `json:"artifacts"`
}

type RecallMediaObject struct {
	ID     string         `json:"id"`
	Kind   string         `json:"kind"`
	Status string         `json:"status"`
	Data   map[string]any `json:"data"`
}

type RecallCalendarMeeting struct {
	ID             string            `json:"id"`
	Platform       string            `json:"platform"`
	MeetingURL     string            `json:"meeting_url"`
	StartsAt       time.Time         `json:"starts_at"`
	BotID          string            `json:"bot_id,omitempty"`
	CalendarUser   string            `json:"calendar_user"`
	RecordingState string            `json:"recording_state"`
	Metadata       map[string]string `json:"metadata,omitempty"`
}

type RecallWebhookSample struct {
	Event       string         `json:"event"`
	Description string         `json:"description"`
	Payload     map[string]any `json:"payload"`
}

type recallCreateBotRequest struct {
	MeetingURL       string            `json:"meeting_url"`
	JoinAt           string            `json:"join_at"`
	BotName          string            `json:"bot_name"`
	RecordingMode    string            `json:"recording_mode"`
	Transcription    string            `json:"transcription"`
	Metadata         map[string]string `json:"metadata"`
	OutputMedia      map[string]any    `json:"output_media"`
	RealTimeFeatures []string          `json:"real_time_features"`
	Config           map[string]any    `json:"config"`
}

type recallUpdateBotRequest struct {
	Status      string            `json:"status"`
	BotName     string            `json:"bot_name"`
	MeetingURL  string            `json:"meeting_url"`
	Metadata    map[string]string `json:"metadata"`
	JoinAt      string            `json:"join_at"`
	ConfigPatch map[string]any    `json:"config_patch"`
}

type recallCreateCalendarMeetingRequest struct {
	Platform     string            `json:"platform"`
	MeetingURL   string            `json:"meeting_url"`
	StartsAt     string            `json:"starts_at"`
	CalendarUser string            `json:"calendar_user"`
	Metadata     map[string]string `json:"metadata"`
}

func NewRecallReviewService(cfg Config) *RecallReviewService {
	svc := &RecallReviewService{
		cfg:              cfg,
		bots:             make(map[string]*RecallReviewBot),
		recordings:       make(map[string]*RecallReviewRecording),
		calendarMeetings: make(map[string]*RecallCalendarMeeting),
		catalog:          buildRecallEndpointCatalog(),
		webhookSamples:   defaultRecallWebhookSamples(),
	}
	svc.bootstrapLocked()
	return svc
}

func buildRecallEndpointCatalog() []RecallEndpointDoc {
	return []RecallEndpointDoc{
		{Category: "Must Test", Name: "Create Bot", Description: "Ponto de entrada principal quando voce mesmo controla agenda, invite e meeting_url. Use ad-hoc ou com join_at para agendar.", Official: "POST /api/v1/bot/", Local: "POST /api/review/recall/bots", DocsURL: "https://docs.recall.ai/reference/bot_create", Resource: "bot", Presentation: "Form principal para provar que a plataforma decide quando o bot entra.", Priority: "must_test"},
		{Category: "Must Test", Name: "Retrieve Bot", Description: "Confirma status do bot, meeting_url, metadata, links e o estado operacional depois de criar/agendar.", Official: "GET /api/v1/bot/{bot_id}/", Local: "GET /api/review/recall/bots/{bot_id}", DocsURL: "https://docs.recall.ai/docs/bot-overview", Resource: "bot", Presentation: "Painel de detalhe do bot selecionado.", Priority: "must_test"},
		{Category: "Must Test", Name: "Stop Recording", Description: "Fecha a captura e gera os artifacts mais relevantes para a POC: transcript, audio, metadata e participant events.", Official: "POST /api/v1/bot/{bot_id}/stop_recording/", Local: "POST /api/review/recall/bots/{bot_id}/stop_recording", DocsURL: "https://docs.recall.ai/docs/media-shortcuts", Resource: "recording", Presentation: "Passo critico para validar ingestao interna.", Priority: "must_test"},
		{Category: "Must Test", Name: "Retrieve Recording", Description: "Permite confirmar quais media_shortcuts e artefatos ficaram disponiveis depois da reuniao.", Official: "GET /api/v1/recording/{recording_id}/", Local: "GET /api/review/recall/recordings/{recording_id}", DocsURL: "https://docs.recall.ai/docs/media-shortcuts", Resource: "recording", Presentation: "Card expandido para inspecionar tudo que a plataforma vai processar.", Priority: "must_test"},
		{Category: "Must Test", Name: "Retrieve Transcript", Description: "Valida o transcript que sera consumido pela plataforma para NLP, sumarizacao ou analytics.", Official: "GET /api/v1/transcript/{transcript_id}/", Local: "GET /api/review/recall/transcripts/{transcript_id}", DocsURL: "https://docs.recall.ai/docs/async-transcription", Resource: "transcript", Presentation: "Preview do transcript na UI.", Priority: "must_test"},
		{Category: "Must Test", Name: "Retrieve Meeting Metadata", Description: "Mostra quem entrou na reuniao, nomes, plataforma e campos derivados de identidade; e onde o email pode ou nao aparecer.", Official: "GET /api/v1/meeting_metadata/{metadata_id}/", Local: "GET /api/review/recall/meeting_metadata/{metadata_id}", DocsURL: "https://docs.recall.ai/docs/meeting-participant-emails", Resource: "meeting_metadata", Presentation: "Resposta direta para o tema 'quem e quem'.", Priority: "must_test"},
		{Category: "Must Test", Name: "Retrieve Participant Events", Description: "Timeline de join/leave e mudancas relevantes para auditoria, troubleshooting e correlacao de participantes.", Official: "GET /api/v1/participant_events/{event_id}/", Local: "GET /api/review/recall/participant_events/{event_id}", DocsURL: "https://docs.recall.ai/docs/meeting-participants-events", Resource: "participant_events", Presentation: "Complementa metadata com historico da chamada.", Priority: "must_test"},
		{Category: "Must Test", Name: "Schedule Bot For Calendar Event", Description: "Fluxo mais aderente para operar com um email/agenda dedicados e deixar a Recall seguir o lifecycle dos eventos.", Official: "POST /api/v2/calendar-events/{id}/bot/", Local: "POST /api/review/recall/calendar/meetings", DocsURL: "https://docs.recall.ai/reference/calendar_events_bot_create", Resource: "calendar_meeting", Presentation: "Mostra a POC com agenda compartilhada e auto-join.", Priority: "must_test"},
		{Category: "Must Test", Name: "Webhook Samples", Description: "Representa os callbacks que a plataforma precisa consumir para mudar estado, baixar artefatos e iniciar processamento interno.", Official: "Webhook events", Local: "GET /api/review/recall/webhooks/samples", DocsURL: "https://docs.recall.ai/docs/getting-started", Resource: "webhook", Presentation: "Usado no review para explicar callbacks.", Priority: "must_test"},
		{Category: "Bots", Name: "List Bots", Description: "Lista bots para acompanhar status, meeting_url, metadata e links para recordings.", Official: "GET /api/v1/bot/", Local: "GET /api/review/recall/bots", DocsURL: "https://docs.recall.ai/v1.10/reference/bot_list", Resource: "bot", Presentation: "Tabela principal da review com filtros simples.", Priority: "important"},
		{Category: "Bots", Name: "Update Bot", Description: "Atualiza meeting_url, metadata, join_at e configuracoes mutaveis.", Official: "PATCH /api/v1/bot/{bot_id}/", Local: "PATCH /api/review/recall/bots/{bot_id}", DocsURL: "https://docs.recall.ai/docs/creating-and-scheduling-bots", Resource: "bot", Presentation: "Acao de editar dados sem recriar o recurso.", Priority: "important"},
		{Category: "Bots", Name: "Delete Scheduled Bot", Description: "Cancela um bot ainda agendado para uma reuniao que nao comecou.", Official: "DELETE /api/v1/bot/{bot_id}/", Local: "DELETE /api/review/recall/bots/{bot_id}", DocsURL: "https://docs.recall.ai/reference/bot_destroy", Resource: "bot", Presentation: "Resposta direta para 'posso deletar uma reuniao?'.", Priority: "important"},
		{Category: "Bots", Name: "Leave Call", Description: "Remove o bot de uma reuniao que ja esta entrando ou em andamento.", Official: "POST /api/v1/bot/{bot_id}/leave_call/", Local: "POST /api/review/recall/bots/{bot_id}/leave", DocsURL: "https://docs.recall.ai/reference/bot_leave_call_create", Resource: "bot", Presentation: "Encerramento limpo da sessao.", Priority: "important"},
		{Category: "Recording", Name: "Start Recording", Description: "Inicia gravacao sob demanda para um bot ja conectado.", Official: "POST /api/v1/bot/{bot_id}/start_recording/", Local: "POST /api/review/recall/bots/{bot_id}/start_recording", DocsURL: "https://docs.recall.ai/docs/media-shortcuts", Resource: "recording", Presentation: "Botao para simular gravacao manual.", Priority: "important"},
		{Category: "Recording", Name: "Pause Recording", Description: "Pausa captura de midia em uma gravacao em andamento.", Official: "POST /api/v1/bot/{bot_id}/pause_recording/", Local: "POST /api/review/recall/bots/{bot_id}/pause_recording", DocsURL: "https://docs.recall.ai/docs/bot-overview", Resource: "recording", Presentation: "Mostra diferenca entre status do bot e da gravacao.", Priority: "important"},
		{Category: "Recording", Name: "Resume Recording", Description: "Retoma uma gravacao pausada.", Official: "POST /api/v1/bot/{bot_id}/resume_recording/", Local: "POST /api/review/recall/bots/{bot_id}/resume_recording", DocsURL: "https://docs.recall.ai/docs/bot-overview", Resource: "recording", Presentation: "Completa o fluxo start/pause/resume/stop.", Priority: "important"},
		{Category: "Recording", Name: "List Recordings", Description: "Lista gravacoes geradas pelos bots e respectivos artefatos.", Official: "GET /api/v1/recording/", Local: "GET /api/review/recall/recordings", DocsURL: "https://docs.recall.ai/docs/media-shortcuts", Resource: "recording", Presentation: "Tabela de gravacoes e status.", Priority: "important"},
		{Category: "Recording", Name: "Delete Bot Media", Description: "Apaga transcript, recordings e metadata armazenados na Recall depois do processamento interno.", Official: "POST /api/v1/bot/{bot_id}/delete_media/", Local: "DELETE /api/review/recall/recordings/{recording_id}", DocsURL: "https://docs.recall.ai/reference/bot_delete_media_create", Resource: "recording", Presentation: "Ponto de governanca e retention para a POC.", Priority: "important"},
		{Category: "Media", Name: "Retrieve Audio Mixed", Description: "Acessa audio consolidado da reuniao para pipeline interno ou backup.", Official: "GET /api/v1/audio_mixed/{audio_id}/", Local: "GET /api/review/recall/audio_mixed/{audio_id}", DocsURL: "https://docs.recall.ai/docs/media-shortcuts", Resource: "audio_mixed", Presentation: "Complementa cenarios audio-only.", Priority: "important"},
		{Category: "Media", Name: "Retrieve Video Mixed", Description: "Acessa video MP4 consolidado da reuniao.", Official: "GET /api/v1/video_mixed/{video_id}/", Local: "GET /api/review/recall/video_mixed/{video_id}", DocsURL: "https://docs.recall.ai/docs/media-shortcuts", Resource: "video_mixed", Presentation: "Link para arquivo servido localmente.", Priority: "important"},
		{Category: "Identity", Name: "Meeting Participant Emails", Description: "Explica a principal limitacao da POC: emails nao vem do provider; so aparecem por matching via Calendar Integration e podem vir nulos.", Official: "Feature doc", Local: "GET /api/review/recall/meeting_metadata/{metadata_id}", DocsURL: "https://docs.recall.ai/docs/meeting-participant-emails", Resource: "identity", Presentation: "Ajuda a explicar confianca de identidade por participante.", Priority: "important"},
		{Category: "Identity", Name: "Perfect Diarization", Description: "Quando for critico saber quem falou o que, a Recall pode usar streams separados por participante em plataformas suportadas.", Official: "recording_config.transcript.diarization.use_separate_streams_when_available", Local: "Config documentada na POC", DocsURL: "https://docs.recall.ai/docs/perfect-diarization", Resource: "transcript", Presentation: "Ponto importante para a camada de processamento interno.", Priority: "important"},
		{Category: "Identity", Name: "Separate Audio Per Participant", Description: "Permite receber audio separado por participante para processamento proprio, inclusive em tempo real.", Official: "recording_config.audio_separate_raw", Local: "Config documentada na POC", DocsURL: "https://docs.recall.ai/docs/how-to-get-separate-audio-per-participant-realtime", Resource: "audio_separate", Presentation: "Opcao para NLP ou diarizacao propria.", Priority: "important"},
		{Category: "Calendar", Name: "List Calendar Meetings", Description: "Lista reunioes monitoradas via calendario.", Official: "GET /api/v2/calendar-events/", Local: "GET /api/review/recall/calendar/meetings", DocsURL: "https://docs.recall.ai/docs/scheduling-guide", Resource: "calendar_meeting", Presentation: "Tabela com starts_at e estado de gravacao.", Priority: "important"},
		{Category: "Calendar", Name: "Retrieve/Update Calendar Meeting", Description: "Permite consultar e ajustar estado/metadados da reuniao de calendario.", Official: "GET|PUT local helper for scheduled event", Local: "GET|PUT /api/review/recall/calendar/meetings/{meeting_id}", DocsURL: "https://docs.recall.ai/docs/scheduling-guide", Resource: "calendar_meeting", Presentation: "Mostra associacao entre agenda e bot.", Priority: "important"},
		{Category: "Calendar", Name: "Unschedule Bot For Calendar Event", Description: "Remove o bot de um evento sincronizado quando a gravacao e desligada ou o evento deixa de existir.", Official: "DELETE /api/v2/calendar-events/{id}/bot/", Local: "PUT /api/review/recall/calendar/meetings/{meeting_id}", DocsURL: "https://docs.recall.ai/reference/calendar_events_bot_destroy", Resource: "calendar_meeting", Presentation: "Explica como desligar futuras capturas.", Priority: "important"},
		{Category: "Calendar", Name: "Refresh Calendar", Description: "Forca sincronizacao do calendario para puxar reunioes futuras.", Official: "Sync via calendar webhooks + fetch events", Local: "POST /api/review/recall/calendar/meetings/refresh", DocsURL: "https://docs.recall.ai/docs/scheduling-guide", Resource: "calendar_meeting", Presentation: "Acao administrativa de sync.", Priority: "important"},
		{Category: "Output Media", Name: "Configure Output Media", Description: "Mantido na POC para mostrar extensibilidade, mas nao e o foco principal desta avaliacao de captura.", Official: "Campo output_media em Create/Update Bot", Local: "POST /api/review/recall/bots/{bot_id}/output_media", DocsURL: "https://docs.recall.ai/v1.10/docs/stream-media", Resource: "output_media", Presentation: "Explica bots que escutam e respondem na reuniao.", Priority: "optional"},
	}
}

func defaultRecallWebhookSamples() []RecallWebhookSample {
	return []RecallWebhookSample{
		{
			Event:       "bot.joined",
			Description: "Bot entrou na chamada e esta pronto para gravar/streamar.",
			Payload: map[string]any{
				"event": "bot.joined",
				"data": map[string]any{
					"bot": map[string]any{"id": "bot_demo_joined", "status": "in_call"},
				},
			},
		},
		{
			Event:       "recording.started",
			Description: "Gravacao iniciada manualmente ou por config automatica.",
			Payload: map[string]any{
				"event": "recording.started",
				"data": map[string]any{
					"recording": map[string]any{"id": "rec_demo_started", "status": "in_progress"},
				},
			},
		},
		{
			Event:       "recording.done",
			Description: "Gravacao concluida com media_shortcuts para consumo posterior.",
			Payload: map[string]any{
				"event": "recording.done",
				"data": map[string]any{
					"recording": map[string]any{"id": "rec_demo_done", "status": "completed"},
					"media_shortcuts": map[string]any{
						"video_mixed": map[string]any{"id": "video_demo_1"},
						"transcript":  map[string]any{"id": "transcript_demo_1"},
					},
				},
			},
		},
		{
			Event:       "bot.left",
			Description: "Bot saiu da reuniao.",
			Payload: map[string]any{
				"event": "bot.left",
				"data": map[string]any{
					"bot": map[string]any{"id": "bot_demo_left", "status": "done"},
				},
			},
		},
	}
}

func (s *RecallReviewService) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bots = make(map[string]*RecallReviewBot)
	s.recordings = make(map[string]*RecallReviewRecording)
	s.calendarMeetings = make(map[string]*RecallCalendarMeeting)
	s.webhookSamples = defaultRecallWebhookSamples()
	s.bootstrapLocked()
}

func (s *RecallReviewService) Bootstrap() RecallReviewState {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bootstrapLocked()
	return s.snapshotLocked()
}

func (s *RecallReviewService) bootstrapLocked() {
	if len(s.bots) > 0 {
		return
	}

	joinedAt := time.Now().UTC().Add(-45 * time.Minute)
	scheduledAt := time.Now().UTC().Add(75 * time.Minute)

	botA := &RecallReviewBot{
		ID:            "bot_demo_review_1",
		Status:        "done",
		MeetingURL:    "https://meet.google.com/demo-review-001",
		RecordingMode: "speaker_view",
		Transcription: "real_time",
		BotName:       "Review Recorder",
		Metadata:      map[string]string{"meeting_id": "mtg-review-001", "owner": "sales-eng"},
		RecordingIDs:  []string{},
		CreatedAt:     joinedAt,
		UpdatedAt:     time.Now().UTC(),
		RealTimeFeatures: []string{
			"captions",
			"speaker_events",
			"real_time_transcript",
		},
		OutputMedia: nil,
		Config: map[string]any{
			"recording":     map[string]any{"video_mixed": true, "audio_mixed": true},
			"transcription": map[string]any{"provider": "deepgram", "mode": "real_time"},
		},
	}
	recordingA := s.newRecordingLocked(botA, "completed")
	recordingA.StartedAt = joinedAt
	ended := joinedAt.Add(37 * time.Minute)
	recordingA.CompletedAt = &ended
	recordingA.DurationSec = int(ended.Sub(joinedAt).Seconds())
	recordingA.MediaShortcuts = s.materializeMediaLocked(recordingA)
	s.recordings[recordingA.ID] = recordingA
	botA.RecordingIDs = append(botA.RecordingIDs, recordingA.ID)
	botA.Links = s.botLinks(botA.ID)
	s.bots[botA.ID] = botA

	botB := &RecallReviewBot{
		ID:            "bot_demo_review_2",
		Status:        "scheduled",
		MeetingURL:    "https://zoom.us/j/999000111",
		JoinAt:        &scheduledAt,
		RecordingMode: "gallery_view_v2",
		Transcription: "async",
		BotName:       "Agent Presenter",
		Metadata:      map[string]string{"meeting_id": "mtg-review-002", "demo": "output-media"},
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
		OutputMedia: map[string]any{
			"camera": map[string]any{
				"kind": "webpage",
				"url":  "https://example.com/avatar-demo",
			},
			"microphone": map[string]any{
				"kind": "webpage_audio",
				"url":  "https://example.com/tts-demo",
			},
		},
		RealTimeFeatures: []string{"output_media", "streaming_agent"},
		Config: map[string]any{
			"automatic_leave": false,
			"transcription":   map[string]any{"provider": "assembly_ai", "mode": "async"},
		},
	}
	botB.Links = s.botLinks(botB.ID)
	s.bots[botB.ID] = botB

	cal := &RecallCalendarMeeting{
		ID:             "cal_demo_review_1",
		Platform:       "google_meet",
		MeetingURL:     botB.MeetingURL,
		StartsAt:       scheduledAt,
		BotID:          botB.ID,
		CalendarUser:   "founders@demo.example",
		RecordingState: "scheduled",
		Metadata:       map[string]string{"source": "calendar-sync", "workspace": "review"},
	}
	s.calendarMeetings[cal.ID] = cal
}

func (s *RecallReviewService) Catalog() []RecallEndpointDoc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RecallEndpointDoc, len(s.catalog))
	copy(out, s.catalog)
	return out
}

func (s *RecallReviewService) State() RecallReviewState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snapshotLocked()
}

func (s *RecallReviewService) snapshotLocked() RecallReviewState {
	bots := make([]RecallReviewBot, 0, len(s.bots))
	recordings := make([]RecallReviewRecording, 0, len(s.recordings))
	calendar := make([]RecallCalendarMeeting, 0, len(s.calendarMeetings))
	var summary RecallReviewSummary

	for _, bot := range s.bots {
		bots = append(bots, *cloneBot(bot))
	}
	for _, rec := range s.recordings {
		recordings = append(recordings, *cloneRecording(rec))
		summary.Recordings++
		if rec.Status == "completed" {
			summary.Completed++
		}
		if rec.Status == "in_progress" || rec.Status == "paused" {
			summary.InProgress++
		}
	}
	for _, m := range s.calendarMeetings {
		calendar = append(calendar, *cloneCalendarMeeting(m))
	}

	summary.Bots = len(bots)
	summary.CalendarMeetings = len(calendar)

	sort.Slice(bots, func(i, j int) bool { return bots[i].CreatedAt.After(bots[j].CreatedAt) })
	sort.Slice(recordings, func(i, j int) bool { return recordings[i].StartedAt.After(recordings[j].StartedAt) })
	sort.Slice(calendar, func(i, j int) bool { return calendar[i].StartsAt.Before(calendar[j].StartsAt) })

	out := RecallReviewState{
		GeneratedAt:       time.Now().UTC(),
		Summary:           summary,
		Bots:              bots,
		Recordings:        recordings,
		CalendarMeetings:  calendar,
		WebhookSamples:    append([]RecallWebhookSample(nil), s.webhookSamples...),
		OfficialDocsEntry: "https://docs.recall.ai/docs/getting-started?utm_source=Recall.ai&utm_medium=navbar&utm_content=API_Docs",
		ReviewHints: []string{
			"Este ambiente local demonstra fluxos e contratos; nao substitui a API oficial.",
			"Os links oficiais no catalogo apontam para a documentacao usada como referencia.",
			"Output Media aparece como configuracao do bot, nao como recurso de midia gravada.",
		},
	}
	return out
}

func (s *RecallReviewService) CreateBot(req recallCreateBotRequest) *RecallReviewBot {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	bot := &RecallReviewBot{
		ID:               fmt.Sprintf("bot_demo_%d", now.UnixNano()),
		Status:           "joining_call",
		MeetingURL:       firstNonEmpty(req.MeetingURL, "https://meet.google.com/review-demo-created"),
		RecordingMode:    firstNonEmpty(req.RecordingMode, "speaker_view"),
		Transcription:    firstNonEmpty(req.Transcription, "real_time"),
		BotName:          firstNonEmpty(req.BotName, "Review Bot"),
		Metadata:         cloneStringMap(req.Metadata),
		OutputMedia:      cloneAnyMap(req.OutputMedia),
		RecordingIDs:     []string{},
		RealTimeFeatures: append([]string(nil), req.RealTimeFeatures...),
		CreatedAt:        now,
		UpdatedAt:        now,
		Config:           cloneAnyMap(req.Config),
	}
	if bot.Metadata == nil {
		bot.Metadata = map[string]string{"meeting_id": generateMeetingID()}
	}
	if strings.TrimSpace(req.JoinAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, req.JoinAt); err == nil {
			bot.JoinAt = &parsed
			bot.Status = "scheduled"
		}
	}
	bot.Links = s.botLinks(bot.ID)
	s.bots[bot.ID] = bot
	return cloneBot(bot)
}

func (s *RecallReviewService) ListBots() []RecallReviewBot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RecallReviewBot, 0, len(s.bots))
	for _, bot := range s.bots {
		out = append(out, *cloneBot(bot))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

func (s *RecallReviewService) GetBot(id string) (*RecallReviewBot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bot, ok := s.bots[id]
	if !ok {
		return nil, false
	}
	return cloneBot(bot), true
}

func (s *RecallReviewService) UpdateBot(id string, req recallUpdateBotRequest) (*RecallReviewBot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, ok := s.bots[id]
	if !ok {
		return nil, false
	}
	if strings.TrimSpace(req.Status) != "" {
		bot.Status = req.Status
	}
	if strings.TrimSpace(req.BotName) != "" {
		bot.BotName = req.BotName
	}
	if strings.TrimSpace(req.MeetingURL) != "" {
		bot.MeetingURL = req.MeetingURL
	}
	if len(req.Metadata) > 0 {
		if bot.Metadata == nil {
			bot.Metadata = map[string]string{}
		}
		for k, v := range req.Metadata {
			bot.Metadata[k] = v
		}
	}
	if strings.TrimSpace(req.JoinAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, req.JoinAt); err == nil {
			bot.JoinAt = &parsed
		}
	}
	if len(req.ConfigPatch) > 0 {
		if bot.Config == nil {
			bot.Config = map[string]any{}
		}
		for k, v := range req.ConfigPatch {
			bot.Config[k] = v
		}
	}
	bot.UpdatedAt = time.Now().UTC()
	return cloneBot(bot), true
}

func (s *RecallReviewService) DeleteBot(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, ok := s.bots[id]
	if !ok {
		return false
	}
	for _, recordingID := range bot.RecordingIDs {
		delete(s.recordings, recordingID)
	}
	for key, cal := range s.calendarMeetings {
		if cal.BotID == id {
			delete(s.calendarMeetings, key)
		}
	}
	delete(s.bots, id)
	return true
}

func (s *RecallReviewService) SetOutputMedia(id string, payload map[string]any) (*RecallReviewBot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, ok := s.bots[id]
	if !ok {
		return nil, false
	}
	bot.OutputMedia = cloneAnyMap(payload)
	bot.UpdatedAt = time.Now().UTC()
	return cloneBot(bot), true
}

func (s *RecallReviewService) ClearOutputMedia(id string) (*RecallReviewBot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, ok := s.bots[id]
	if !ok {
		return nil, false
	}
	bot.OutputMedia = nil
	bot.UpdatedAt = time.Now().UTC()
	return cloneBot(bot), true
}

func (s *RecallReviewService) StartRecording(botID string) (*RecallReviewRecording, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, ok := s.bots[botID]
	if !ok {
		return nil, fmt.Errorf("bot not found")
	}
	for _, recordingID := range bot.RecordingIDs {
		if rec, exists := s.recordings[recordingID]; exists && (rec.Status == "in_progress" || rec.Status == "paused") {
			return nil, fmt.Errorf("recording already active")
		}
	}
	rec := s.newRecordingLocked(bot, "in_progress")
	s.recordings[rec.ID] = rec
	bot.RecordingIDs = append([]string{rec.ID}, bot.RecordingIDs...)
	bot.Status = "in_call_recording"
	bot.UpdatedAt = time.Now().UTC()
	return cloneRecording(rec), nil
}

func (s *RecallReviewService) PauseRecording(botID string) (*RecallReviewRecording, error) {
	return s.transitionRecording(botID, "paused")
}

func (s *RecallReviewService) ResumeRecording(botID string) (*RecallReviewRecording, error) {
	return s.transitionRecording(botID, "in_progress")
}

func (s *RecallReviewService) StopRecording(botID string) (*RecallReviewRecording, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, rec, err := s.getActiveRecordingLocked(botID)
	if err != nil {
		return nil, err
	}
	completedAt := time.Now().UTC()
	rec.Status = "completed"
	rec.CompletedAt = &completedAt
	rec.DurationSec = int(completedAt.Sub(rec.StartedAt).Seconds())
	if rec.DurationSec <= 0 {
		rec.DurationSec = 900
	}
	rec.MediaShortcuts = s.materializeMediaLocked(rec)
	bot.Status = "done"
	bot.UpdatedAt = completedAt
	return cloneRecording(rec), nil
}

func (s *RecallReviewService) LeaveCall(botID string) (*RecallReviewBot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, ok := s.bots[botID]
	if !ok {
		return nil, fmt.Errorf("bot not found")
	}
	for _, recordingID := range bot.RecordingIDs {
		rec, exists := s.recordings[recordingID]
		if !exists {
			continue
		}
		if rec.Status == "in_progress" || rec.Status == "paused" {
			completedAt := time.Now().UTC()
			rec.Status = "completed"
			rec.CompletedAt = &completedAt
			rec.DurationSec = int(completedAt.Sub(rec.StartedAt).Seconds())
			rec.MediaShortcuts = s.materializeMediaLocked(rec)
			break
		}
	}
	bot.Status = "done"
	bot.UpdatedAt = time.Now().UTC()
	return cloneBot(bot), nil
}

func (s *RecallReviewService) ListRecordings() []RecallReviewRecording {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RecallReviewRecording, 0, len(s.recordings))
	for _, rec := range s.recordings {
		out = append(out, *cloneRecording(rec))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out
}

func (s *RecallReviewService) GetRecording(id string) (*RecallReviewRecording, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec, ok := s.recordings[id]
	if !ok {
		return nil, false
	}
	return cloneRecording(rec), true
}

func (s *RecallReviewService) DeleteRecording(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.recordings[id]
	if !ok {
		return false
	}
	delete(s.recordings, id)
	if bot, exists := s.bots[rec.BotID]; exists {
		filtered := bot.RecordingIDs[:0]
		for _, recordingID := range bot.RecordingIDs {
			if recordingID != id {
				filtered = append(filtered, recordingID)
			}
		}
		bot.RecordingIDs = filtered
		bot.UpdatedAt = time.Now().UTC()
	}
	return true
}

func (s *RecallReviewService) GetMedia(kind, id string) (RecallMediaObject, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rec := range s.recordings {
		for _, media := range rec.MediaShortcuts {
			if media.Kind == kind && media.ID == id {
				return media, true
			}
		}
	}
	return RecallMediaObject{}, false
}

func (s *RecallReviewService) ListCalendarMeetings() []RecallCalendarMeeting {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RecallCalendarMeeting, 0, len(s.calendarMeetings))
	for _, meeting := range s.calendarMeetings {
		out = append(out, *cloneCalendarMeeting(meeting))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	return out
}

func (s *RecallReviewService) CreateCalendarMeeting(req recallCreateCalendarMeetingRequest) *RecallCalendarMeeting {
	s.mu.Lock()
	defer s.mu.Unlock()
	startsAt := time.Now().UTC().Add(2 * time.Hour)
	if strings.TrimSpace(req.StartsAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, req.StartsAt); err == nil {
			startsAt = parsed
		}
	}
	meeting := &RecallCalendarMeeting{
		ID:             fmt.Sprintf("cal_demo_%d", time.Now().UTC().UnixNano()),
		Platform:       firstNonEmpty(req.Platform, "google_meet"),
		MeetingURL:     firstNonEmpty(req.MeetingURL, "https://meet.google.com/review-calendar-created"),
		StartsAt:       startsAt,
		CalendarUser:   firstNonEmpty(req.CalendarUser, "owner@demo.example"),
		RecordingState: "scheduled",
		Metadata:       cloneStringMap(req.Metadata),
	}
	s.calendarMeetings[meeting.ID] = meeting
	return cloneCalendarMeeting(meeting)
}

func (s *RecallReviewService) GetCalendarMeeting(id string) (*RecallCalendarMeeting, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	meeting, ok := s.calendarMeetings[id]
	if !ok {
		return nil, false
	}
	return cloneCalendarMeeting(meeting), true
}

func (s *RecallReviewService) UpdateCalendarMeeting(id string, req recallCreateCalendarMeetingRequest) (*RecallCalendarMeeting, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	meeting, ok := s.calendarMeetings[id]
	if !ok {
		return nil, false
	}
	if strings.TrimSpace(req.Platform) != "" {
		meeting.Platform = req.Platform
	}
	if strings.TrimSpace(req.MeetingURL) != "" {
		meeting.MeetingURL = req.MeetingURL
	}
	if strings.TrimSpace(req.CalendarUser) != "" {
		meeting.CalendarUser = req.CalendarUser
	}
	if len(req.Metadata) > 0 {
		if meeting.Metadata == nil {
			meeting.Metadata = map[string]string{}
		}
		for k, v := range req.Metadata {
			meeting.Metadata[k] = v
		}
	}
	if strings.TrimSpace(req.StartsAt) != "" {
		if parsed, err := time.Parse(time.RFC3339, req.StartsAt); err == nil {
			meeting.StartsAt = parsed
		}
	}
	return cloneCalendarMeeting(meeting), true
}

func (s *RecallReviewService) RefreshCalendar() RecallReviewState {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, meeting := range s.calendarMeetings {
		if meeting.StartsAt.After(time.Now().UTC()) {
			meeting.RecordingState = "refreshed"
		}
	}
	return s.snapshotLocked()
}

func (s *RecallReviewService) transitionRecording(botID, target string) (*RecallReviewRecording, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bot, rec, err := s.getActiveRecordingLocked(botID)
	if err != nil {
		return nil, err
	}
	rec.Status = target
	bot.UpdatedAt = time.Now().UTC()
	if target == "paused" {
		bot.Status = "recording_paused"
	} else {
		bot.Status = "in_call_recording"
	}
	return cloneRecording(rec), nil
}

func (s *RecallReviewService) getActiveRecordingLocked(botID string) (*RecallReviewBot, *RecallReviewRecording, error) {
	bot, ok := s.bots[botID]
	if !ok {
		return nil, nil, fmt.Errorf("bot not found")
	}
	for _, recordingID := range bot.RecordingIDs {
		rec, exists := s.recordings[recordingID]
		if exists && (rec.Status == "in_progress" || rec.Status == "paused") {
			return bot, rec, nil
		}
	}
	return nil, nil, fmt.Errorf("active recording not found")
}

func (s *RecallReviewService) newRecordingLocked(bot *RecallReviewBot, status string) *RecallReviewRecording {
	now := time.Now().UTC()
	return &RecallReviewRecording{
		ID:          fmt.Sprintf("rec_demo_%d", now.UnixNano()),
		BotID:       bot.ID,
		Status:      status,
		StartedAt:   now,
		DurationSec: 0,
		MediaShortcuts: map[string]RecallMediaObject{
			"video_mixed":        {},
			"transcript":         {},
			"audio_mixed":        {},
			"meeting_metadata":   {},
			"participant_events": {},
		},
		Artifacts: map[string]string{},
	}
}

func (s *RecallReviewService) materializeMediaLocked(rec *RecallReviewRecording) map[string]RecallMediaObject {
	downloadRoot := filepath.Join(s.cfg.DataDir, "review")
	_ = os.MkdirAll(downloadRoot, 0o755)

	videoID := rec.ID + "_video"
	audioID := rec.ID + "_audio"
	transcriptID := rec.ID + "_transcript"
	metadataID := rec.ID + "_metadata"
	eventsID := rec.ID + "_events"

	videoPath := filepath.Join(downloadRoot, videoID+".mp4")
	audioPath := filepath.Join(downloadRoot, audioID+".mp3")
	transcriptPath := filepath.Join(downloadRoot, transcriptID+".json")
	metadataPath := filepath.Join(downloadRoot, metadataID+".json")
	eventsPath := filepath.Join(downloadRoot, eventsID+".json")

	_ = os.WriteFile(videoPath, []byte("mock mixed video for recall review\n"), 0o644)
	_ = os.WriteFile(audioPath, []byte("mock mixed audio for recall review\n"), 0o644)

	transcriptBody := map[string]any{
		"utterances": []map[string]any{
			{"speaker": "Mateus", "start": 0, "end": 18, "text": "Abertura do review da Recall."},
			{"speaker": "Ana", "start": 19, "end": 62, "text": "Mostrando create bot, recordings e media shortcuts."},
		},
	}
	metadataBody := map[string]any{
		"platform": "google_meet",
		"calendar_context": map[string]any{
			"calendar_user":       "recall-poc@demo.example",
			"recording_via":       "calendar_integration_v2",
			"identity_resolution": "fuzzy_match_between_invitees_and_in_call_participants",
		},
		"participants": []map[string]any{
			{"name": "Mateus Dreher", "role": "host", "email": "mateus@demo.example", "email_source": "calendar_match", "participant_id": 1},
			{"name": "Ana Silva", "role": "guest", "email": "ana@demo.example", "email_source": "calendar_match", "participant_id": 2},
			{"name": "Guest Mobile", "role": "guest", "email": nil, "email_source": "unmatched", "participant_id": 3},
		},
		"duration_sec": rec.DurationSec,
		"notes": []string{
			"Participant emails nao sao expostos diretamente pelo provedor da call.",
			"Na Recall, o email pode ser preenchido por matching com o calendario quando essa feature estiver habilitada.",
		},
	}
	eventsBody := map[string]any{
		"events": []map[string]any{
			{"type": "participant_joined", "at_sec": 0, "participant": "Mateus Dreher", "participant_id": 1},
			{"type": "participant_joined", "at_sec": 1, "participant": "Ana Silva", "participant_id": 2},
			{"type": "participant_joined", "at_sec": 44, "participant": "Guest Mobile", "participant_id": 3},
			{"type": "participant_left", "at_sec": rec.DurationSec, "participant": "Ana Silva", "participant_id": 2},
		},
	}

	writePrettyJSON(transcriptPath, transcriptBody)
	writePrettyJSON(metadataPath, metadataBody)
	writePrettyJSON(eventsPath, eventsBody)

	base := s.cfg.PublicBaseURL + "/static/review/"
	rec.Artifacts = map[string]string{
		"video_mixed":        base + filepath.Base(videoPath),
		"audio_mixed":        base + filepath.Base(audioPath),
		"transcript":         base + filepath.Base(transcriptPath),
		"meeting_metadata":   base + filepath.Base(metadataPath),
		"participant_events": base + filepath.Base(eventsPath),
	}

	return map[string]RecallMediaObject{
		"video_mixed": {
			ID:     videoID,
			Kind:   "video_mixed",
			Status: "completed",
			Data:   map[string]any{"download_url": rec.Artifacts["video_mixed"]},
		},
		"audio_mixed": {
			ID:     audioID,
			Kind:   "audio_mixed",
			Status: "completed",
			Data:   map[string]any{"download_url": rec.Artifacts["audio_mixed"]},
		},
		"transcript": {
			ID:     transcriptID,
			Kind:   "transcript",
			Status: "completed",
			Data:   map[string]any{"download_url": rec.Artifacts["transcript"]},
		},
		"meeting_metadata": {
			ID:     metadataID,
			Kind:   "meeting_metadata",
			Status: "completed",
			Data:   map[string]any{"download_url": rec.Artifacts["meeting_metadata"]},
		},
		"participant_events": {
			ID:     eventsID,
			Kind:   "participant_events",
			Status: "completed",
			Data:   map[string]any{"download_url": rec.Artifacts["participant_events"]},
		},
	}
}

func (s *RecallReviewService) botLinks(id string) map[string]string {
	return map[string]string{
		"self":            s.cfg.PublicBaseURL + "/api/review/recall/bots/" + id,
		"start_recording": s.cfg.PublicBaseURL + "/api/review/recall/bots/" + id + "/start_recording",
		"stop_recording":  s.cfg.PublicBaseURL + "/api/review/recall/bots/" + id + "/stop_recording",
		"leave":           s.cfg.PublicBaseURL + "/api/review/recall/bots/" + id + "/leave",
	}
}

func writePrettyJSON(path string, value any) {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, body, 0o644)
}

func cloneBot(bot *RecallReviewBot) *RecallReviewBot {
	cp := *bot
	cp.Metadata = cloneStringMap(bot.Metadata)
	cp.OutputMedia = cloneAnyMap(bot.OutputMedia)
	cp.RecordingIDs = append([]string(nil), bot.RecordingIDs...)
	cp.RealTimeFeatures = append([]string(nil), bot.RealTimeFeatures...)
	cp.Links = cloneStringMap(bot.Links)
	cp.Config = cloneAnyMap(bot.Config)
	return &cp
}

func cloneRecording(rec *RecallReviewRecording) *RecallReviewRecording {
	cp := *rec
	cp.MediaShortcuts = make(map[string]RecallMediaObject, len(rec.MediaShortcuts))
	for k, v := range rec.MediaShortcuts {
		cp.MediaShortcuts[k] = RecallMediaObject{
			ID:     v.ID,
			Kind:   v.Kind,
			Status: v.Status,
			Data:   cloneAnyMap(v.Data),
		}
	}
	cp.Artifacts = cloneStringMap(rec.Artifacts)
	return &cp
}

func cloneCalendarMeeting(meeting *RecallCalendarMeeting) *RecallCalendarMeeting {
	cp := *meeting
	cp.Metadata = cloneStringMap(meeting.Metadata)
	return &cp
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (a *App) recallCatalogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": a.recallReview.Catalog()})
}

func (a *App) recallStateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.recallReview.State())
}

func (a *App) recallResetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.recallReview.Reset()
	writeJSON(w, http.StatusOK, a.recallReview.State())
}

func (a *App) recallBootstrapHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.recallReview.Bootstrap())
}

func (a *App) recallWebhookSamplesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"samples": a.recallReview.State().WebhookSamples})
}

func (a *App) recallBotsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"results": a.recallReview.ListBots()})
	case http.MethodPost:
		var req recallCreateBotRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, a.recallReview.CreateBot(req))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) recallBotByIDHandler(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/review/recall/bots/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "missing bot id", http.StatusBadRequest)
		return
	}
	botID := parts[0]
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			bot, ok := a.recallReview.GetBot(botID)
			if !ok {
				http.Error(w, "bot not found", http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, bot)
		case http.MethodPatch:
			var req recallUpdateBotRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			bot, ok := a.recallReview.UpdateBot(botID, req)
			if !ok {
				http.Error(w, "bot not found", http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, bot)
		case http.MethodDelete:
			if !a.recallReview.DeleteBot(botID) {
				http.Error(w, "bot not found", http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "bot_id": botID})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	action := parts[1]
	switch action {
	case "start_recording":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rec, err := a.recallReview.StartRecording(botID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, rec)
	case "pause_recording":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rec, err := a.recallReview.PauseRecording(botID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, rec)
	case "resume_recording":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rec, err := a.recallReview.ResumeRecording(botID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, rec)
	case "stop_recording":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		rec, err := a.recallReview.StopRecording(botID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, rec)
	case "leave":
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		bot, err := a.recallReview.LeaveCall(botID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, bot)
	case "output_media":
		switch r.Method {
		case http.MethodPost:
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, "invalid json", http.StatusBadRequest)
				return
			}
			bot, ok := a.recallReview.SetOutputMedia(botID, payload)
			if !ok {
				http.Error(w, "bot not found", http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, bot)
		case http.MethodDelete:
			bot, ok := a.recallReview.ClearOutputMedia(botID)
			if !ok {
				http.Error(w, "bot not found", http.StatusNotFound)
				return
			}
			writeJSON(w, http.StatusOK, bot)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		http.Error(w, "unknown action", http.StatusNotFound)
	}
}

func (a *App) recallRecordingsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": a.recallReview.ListRecordings()})
}

func (a *App) recallRecordingByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/review/recall/recordings/")
	if id == "" {
		http.Error(w, "missing recording id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		rec, ok := a.recallReview.GetRecording(id)
		if !ok {
			http.Error(w, "recording not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, rec)
	case http.MethodDelete:
		if !a.recallReview.DeleteRecording(id) {
			http.Error(w, "recording not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "recording_id": id})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) recallTranscriptHandler(w http.ResponseWriter, r *http.Request) {
	a.recallMediaHandler(w, r, "transcript", "/api/review/recall/transcripts/")
}

func (a *App) recallVideoMixedHandler(w http.ResponseWriter, r *http.Request) {
	a.recallMediaHandler(w, r, "video_mixed", "/api/review/recall/video_mixed/")
}

func (a *App) recallAudioMixedHandler(w http.ResponseWriter, r *http.Request) {
	a.recallMediaHandler(w, r, "audio_mixed", "/api/review/recall/audio_mixed/")
}

func (a *App) recallMeetingMetadataHandler(w http.ResponseWriter, r *http.Request) {
	a.recallMediaHandler(w, r, "meeting_metadata", "/api/review/recall/meeting_metadata/")
}

func (a *App) recallParticipantEventsHandler(w http.ResponseWriter, r *http.Request) {
	a.recallMediaHandler(w, r, "participant_events", "/api/review/recall/participant_events/")
}

func (a *App) recallMediaHandler(w http.ResponseWriter, r *http.Request, kind, prefix string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, prefix)
	if id == "" {
		http.Error(w, "missing media id", http.StatusBadRequest)
		return
	}
	media, ok := a.recallReview.GetMedia(kind, id)
	if !ok {
		http.Error(w, "media not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, media)
}

func (a *App) recallCalendarMeetingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"results": a.recallReview.ListCalendarMeetings()})
	case http.MethodPost:
		var req recallCreateCalendarMeetingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, a.recallReview.CreateCalendarMeeting(req))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) recallCalendarMeetingByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/review/recall/calendar/meetings/")
	if id == "" {
		http.Error(w, "missing meeting id", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		meeting, ok := a.recallReview.GetCalendarMeeting(id)
		if !ok {
			http.Error(w, "calendar meeting not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, meeting)
	case http.MethodPut:
		var req recallCreateCalendarMeetingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		meeting, ok := a.recallReview.UpdateCalendarMeeting(id, req)
		if !ok {
			http.Error(w, "calendar meeting not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, meeting)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) recallCalendarRefreshHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, a.recallReview.RefreshCalendar())
}
