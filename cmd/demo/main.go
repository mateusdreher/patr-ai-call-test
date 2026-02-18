package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Port                string
	RecallWebhookSecret string
	PublicBaseURL       string
	DataDir             string
	GeminiAPIKey        string
	GeminiModel         string
}

type App struct {
	cfg            Config
	store          *Store
	queue          chan MeetingEvent
	recallClient   RecallClient
	azureClient    AzureClient
	vertexClient   VertexClient
	mediaFactory   MediaFactory
	deliveryClient DeliveryClient
}

type MeetingSource string

const (
	SourceOnline     MeetingSource = "online"
	SourcePresencial MeetingSource = "presencial"
)

type MeetingStatus string

const (
	StatusQueued        MeetingStatus = "queued"
	StatusWaitingUpload MeetingStatus = "waiting_upload"
	StatusProcessing    MeetingStatus = "processing"
	StatusCompleted     MeetingStatus = "completed"
	StatusFailed        MeetingStatus = "failed"
)

type Meeting struct {
	ID            string         `json:"id"`
	Source        MeetingSource  `json:"source"`
	Status        MeetingStatus  `json:"status"`
	DurationSec   int            `json:"duration_sec"`
	RawObjectPath string         `json:"raw_object_path"`
	RawVideoURL   string         `json:"raw_video_url,omitempty"`
	TranscriptURL string         `json:"transcript_url,omitempty"`
	Transcript    []Utterance    `json:"transcript,omitempty"`
	Topics        []TopicSegment `json:"topics,omitempty"`
	FinalPayload  *FinalPayload  `json:"final_payload,omitempty"`
	Error         string         `json:"error,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type Utterance struct {
	StartSec int    `json:"start_sec"`
	EndSec   int    `json:"end_sec"`
	Speaker  string `json:"speaker"`
	Text     string `json:"text"`
}

type TopicSegment struct {
	Title      string `json:"title"`
	StartSec   int    `json:"start_sec"`
	EndSec     int    `json:"end_sec"`
	ClipURL    string `json:"clip_url,omitempty"`
	FrameURL   string `json:"frame_url,omitempty"`
	FrameNotes string `json:"frame_notes,omitempty"`
}

type FinalPayload struct {
	MeetingID    string         `json:"meeting_id"`
	Source       MeetingSource  `json:"source"`
	DurationSec  int            `json:"duration_sec"`
	Transcript   []Utterance    `json:"transcript"`
	Topics       []TopicSegment `json:"topics"`
	GeneratedAt  time.Time      `json:"generated_at"`
	DemoWarnings []string       `json:"demo_warnings"`
}

type MeetingEvent struct {
	MeetingID string
}

// PARTE 1 (RECALL): formato simplificado legado mantido para fallback de compatibilidade.
type legacyRecallWebhookEvent struct {
	MeetingID       string `json:"meeting_id"`
	MP4URL          string `json:"mp4_url"`
	TranscriptURL   string `json:"transcript_url"`
	DurationSeconds int    `json:"duration_seconds"`
}

// PARTE 1 (RECALL): envelope mockado no estilo webhook oficial (event + data + media_shortcuts).
type recallWebhookEnvelope struct {
	Event string `json:"event"`
	Data  struct {
		Bot struct {
			ID       string `json:"id"`
			Metadata struct {
				MeetingID string `json:"meeting_id"`
			} `json:"metadata"`
		} `json:"bot"`
		Recording struct {
			ID       string  `json:"id"`
			Duration float64 `json:"duration"`
		} `json:"recording"`
		MediaShortcuts struct {
			Video struct {
				Data struct {
					DownloadURL string `json:"download_url"`
				} `json:"data"`
			} `json:"video_mixed"`
			Transcript struct {
				Data struct {
					DownloadURL string `json:"download_url"`
				} `json:"data"`
			} `json:"transcript"`
		} `json:"media_shortcuts"`
	} `json:"data"`
}

type normalizedRecallIngestion struct {
	MeetingID       string
	MP4URL          string
	TranscriptURL   string
	DurationSeconds int
}

// PARTE 3 (NORMALIZAÇÃO): estrutura mock do transcript da Recall no formato "utterances".
type recallTranscriptResponse struct {
	Utterances []struct {
		StartSec float64 `json:"start"`
		EndSec   float64 `json:"end"`
		Speaker  string  `json:"speaker"`
		Text     string  `json:"text"`
	} `json:"utterances"`
}

// PARTE 2 (AZURE): estrutura mock baseada no Fast Transcription.
type azureFastTranscription struct {
	RecognizedPhrases []struct {
		Speaker              int   `json:"speaker"`
		OffsetMilliseconds   int64 `json:"offsetMilliseconds"`
		DurationMilliseconds int64 `json:"durationMilliseconds"`
		NBest                []struct {
			Display string `json:"display"`
		} `json:"nBest"`
	} `json:"recognizedPhrases"`
}

type SignedURLRequest struct {
	MeetingID string `json:"meeting_id"`
	Filename  string `json:"filename"`
}

type SignedURLResponse struct {
	MeetingID   string    `json:"meeting_id"`
	ObjectPath  string    `json:"object_path"`
	UploadURL   string    `json:"upload_url"`
	ExpiresAt   time.Time `json:"expires_at"`
	HowToFinish string    `json:"how_to_finish"`
}

type UploadCompleteRequest struct {
	MeetingID string `json:"meeting_id"`
}

type DemoRunRequest struct {
	Source string `json:"source"`
}

type RecallSampleResponse struct {
	Endpoint string            `json:"endpoint"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	Payload  map[string]any    `json:"payload"`
	Curl     string            `json:"curl"`
	Notes    []string          `json:"notes"`
}

type Store struct {
	mu       sync.RWMutex
	meetings map[string]*Meeting
}

func NewStore() *Store {
	return &Store{meetings: make(map[string]*Meeting)}
}

func (s *Store) Save(m *Meeting) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *m
	s.meetings[m.ID] = &cp
}

func (s *Store) Get(id string) (*Meeting, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.meetings[id]
	if !ok {
		return nil, false
	}
	cp := *m
	return &cp, true
}

type RecallClient interface {
	FetchTranscript(ctx context.Context, meeting *Meeting) ([]Utterance, error)
}

type AzureClient interface {
	TranscribeWithSpeakerID(ctx context.Context, meeting *Meeting) ([]Utterance, error)
}

type VertexClient interface {
	SegmentTopics(ctx context.Context, transcript []Utterance) ([]TopicSegment, error)
}

type MediaFactory interface {
	PrepareRawFromRecall(ctx context.Context, meetingID, sourceURL string) (string, error)
	PrepareRawFromPresencial(ctx context.Context, meetingID string) (string, error)
	GenerateTopicAssets(ctx context.Context, meetingID, rawObjectPath string, topics []TopicSegment) ([]TopicSegment, error)
}

type DeliveryClient interface {
	Deliver(ctx context.Context, payload *FinalPayload) error
}

type mockRecallClient struct{}
type mockAzureClient struct{}
type mockVertexClient struct{}
type mockMediaFactory struct {
	baseDir       string
	publicBaseURL string
}
type mockDeliveryClient struct{}
type geminiAPIClient struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func (m *mockRecallClient) FetchTranscript(_ context.Context, meeting *Meeting) ([]Utterance, error) {
	// PARTE 1 (RECALL): mock bruto no estilo retorno de transcript por utterances.
	// TODO(prod): baixar transcript oficial da Recall.ai (com auth) usando meeting.TranscriptURL.
	raw := recallTranscriptResponse{
		Utterances: []struct {
			StartSec float64 `json:"start"`
			EndSec   float64 `json:"end"`
			Speaker  string  `json:"speaker"`
			Text     string  `json:"text"`
		}{
			{StartSec: 0, EndSec: 24, Speaker: "Mateus", Text: "Bom dia, vamos começar com o contexto do deal."},
			{StartSec: 25, EndSec: 88, Speaker: "Ana", Text: "Trouxe os indicadores de receita e burn rate do trimestre."},
			{StartSec: 89, EndSec: 155, Speaker: "Mateus", Text: "Vamos discutir valuation, riscos e próximos passos."},
		},
	}
	// PARTE 3 (NORMALIZAÇÃO): converte schema Recall -> transcript unificado.
	return normalizeRecallTranscript(raw), nil
}

func (m *mockAzureClient) TranscribeWithSpeakerID(_ context.Context, meeting *Meeting) ([]Utterance, error) {
	// PARTE 2 (AZURE): mock bruto inspirado no resultado de Fast Transcription.
	// TODO(prod): chamar API/SDK Azure Speech e usar payload real retornado (async ou batch).
	raw := azureFastTranscription{
		RecognizedPhrases: []struct {
			Speaker              int   `json:"speaker"`
			OffsetMilliseconds   int64 `json:"offsetMilliseconds"`
			DurationMilliseconds int64 `json:"durationMilliseconds"`
			NBest                []struct {
				Display string `json:"display"`
			} `json:"nBest"`
		}{
			{
				Speaker:              1,
				OffsetMilliseconds:   0,
				DurationMilliseconds: 30000,
				NBest: []struct {
					Display string `json:"display"`
				}{{Display: "Fechando a porta da sala, iniciando discussão sobre pipeline comercial."}},
			},
			{
				Speaker:              2,
				OffsetMilliseconds:   31000,
				DurationMilliseconds: 59000,
				NBest: []struct {
					Display string `json:"display"`
				}{{Display: "No presencial vimos resistência no preço e objeções técnicas."}},
			},
			{
				Speaker:              1,
				OffsetMilliseconds:   91000,
				DurationMilliseconds: 79000,
				NBest: []struct {
					Display string `json:"display"`
				}{{Display: "Vamos consolidar plano de ação e responsáveis por follow up."}},
			},
		},
	}
	// PARTE 3 (NORMALIZAÇÃO): converte schema Azure -> transcript unificado.
	return normalizeAzureFastTranscript(raw), nil
}

func (m *mockVertexClient) SegmentTopics(_ context.Context, transcript []Utterance) ([]TopicSegment, error) {
	// TODO(prod): substituir por chamada Vertex AI Gemini (prompt + schema JSON + validação).
	if len(transcript) == 0 {
		return nil, nil
	}
	var topics []TopicSegment
	first := transcript[0]
	topics = append(topics, TopicSegment{
		Title:    "Contexto Inicial",
		StartSec: first.StartSec,
		EndSec:   first.EndSec,
	})
	if len(transcript) > 1 {
		second := transcript[1]
		topics = append(topics, TopicSegment{
			Title:    "Métricas e Objeções",
			StartSec: second.StartSec,
			EndSec:   second.EndSec,
		})
	}
	last := transcript[len(transcript)-1]
	topics = append(topics, TopicSegment{
		Title:    "Encaminhamentos",
		StartSec: last.StartSec,
		EndSec:   last.EndSec,
	})
	return topics, nil
}

func (m *mockMediaFactory) PrepareRawFromRecall(_ context.Context, meetingID, sourceURL string) (string, error) {
	rawDir := filepath.Join(m.baseDir, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return "", err
	}
	objectPath := filepath.Join("raw", fmt.Sprintf("%s.mp4", meetingID))
	abs := filepath.Join(m.baseDir, objectPath)
	content := fmt.Sprintf("MOCK RAW VIDEO from Recall URL: %s\n", sourceURL)
	return objectPath, os.WriteFile(abs, []byte(content), 0o644)
}

func (m *mockMediaFactory) PrepareRawFromPresencial(_ context.Context, meetingID string) (string, error) {
	rawDir := filepath.Join(m.baseDir, "raw")
	if err := os.MkdirAll(rawDir, 0o755); err != nil {
		return "", err
	}
	objectPath := filepath.Join("raw", fmt.Sprintf("%s.mp4", meetingID))
	abs := filepath.Join(m.baseDir, objectPath)
	if _, err := os.Stat(abs); err == nil {
		return objectPath, nil
	}
	content := "MOCK RAW VIDEO from presencial upload\n"
	return objectPath, os.WriteFile(abs, []byte(content), 0o644)
}

func (m *mockMediaFactory) GenerateTopicAssets(_ context.Context, meetingID, rawObjectPath string, topics []TopicSegment) ([]TopicSegment, error) {
	// TODO(prod): executar ffmpeg via os/exec para cortar clips reais e extrair frames reais.
	_ = rawObjectPath
	clipsDir := filepath.Join(m.baseDir, "clips")
	framesDir := filepath.Join(m.baseDir, "frames")
	if err := os.MkdirAll(clipsDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return nil, err
	}

	ffmpegExists := exec.Command("sh", "-lc", "command -v ffmpeg >/dev/null 2>&1").Run() == nil
	out := make([]TopicSegment, 0, len(topics))

	for i, t := range topics {
		clipName := fmt.Sprintf("%s-topic-%d.mp4", meetingID, i+1)
		frameName := fmt.Sprintf("%s-topic-%d.jpg", meetingID, i+1)
		clipAbs := filepath.Join(clipsDir, clipName)
		frameAbs := filepath.Join(framesDir, frameName)

		clipContent := fmt.Sprintf("Mock clip for [%d-%d] %s | ffmpeg_available=%t\n", t.StartSec, t.EndSec, t.Title, ffmpegExists)
		if err := os.WriteFile(clipAbs, []byte(clipContent), 0o644); err != nil {
			return nil, err
		}

		frameContent := fmt.Sprintf("Mock frame capture @%ds for topic %s\n", t.StartSec, t.Title)
		if err := os.WriteFile(frameAbs, []byte(frameContent), 0o644); err != nil {
			return nil, err
		}

		t.ClipURL = fmt.Sprintf("%s/static/clips/%s", m.publicBaseURL, clipName)
		t.FrameURL = fmt.Sprintf("%s/static/frames/%s", m.publicBaseURL, frameName)
		t.FrameNotes = "Frame representativo mockado para demonstrar vínculo fala+tela."
		out = append(out, t)
	}
	return out, nil
}

func (m *mockDeliveryClient) Deliver(_ context.Context, payload *FinalPayload) error {
	// TODO(prod): entregar via Pub/Sub processed-meetings ou HTTP POST autenticado para o serviço de deals.
	log.Printf("mock delivery meeting=%s topics=%d transcript_lines=%d", payload.MeetingID, len(payload.Topics), len(payload.Transcript))
	return nil
}

func main() {
	cfg := loadConfig()
	vertexClient := VertexClient(&mockVertexClient{})
	if cfg.GeminiAPIKey != "" {
		vertexClient = &geminiAPIClient{
			apiKey: cfg.GeminiAPIKey,
			model:  cfg.GeminiModel,
			httpClient: &http.Client{
				Timeout: 45 * time.Second,
			},
		}
		log.Printf("topic engine using Gemini model=%s", cfg.GeminiModel)
	} else {
		log.Printf("topic engine using mock (set GEMINI_API_KEY to use real Gemini)")
	}

	app := &App{
		cfg:            cfg,
		store:          NewStore(),
		queue:          make(chan MeetingEvent, 128),
		recallClient:   &mockRecallClient{},
		azureClient:    &mockAzureClient{},
		vertexClient:   vertexClient,
		mediaFactory:   &mockMediaFactory{baseDir: cfg.DataDir, publicBaseURL: cfg.PublicBaseURL},
		deliveryClient: &mockDeliveryClient{},
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("failed creating data dir: %v", err)
	}

	go app.workerLoop()
	app.routes()

	addr := ":" + cfg.Port
	log.Printf("demo server listening on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func loadConfig() Config {
	return Config{
		Port:                envOrDefault("PORT", "8080"),
		RecallWebhookSecret: envOrDefault("RECALL_WEBHOOK_SECRET", "demo-secret"),
		PublicBaseURL:       envOrDefault("PUBLIC_BASE_URL", "http://localhost:8080"),
		DataDir:             envOrDefault("DATA_DIR", "./data"),
		GeminiAPIKey:        os.Getenv("GEMINI_API_KEY"),
		GeminiModel:         envOrDefault("GEMINI_MODEL", "gemini-1.5-pro"),
	}
}

func envOrDefault(k, d string) string {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	return v
}

func (a *App) routes() {
	http.HandleFunc("/health", a.healthHandler)
	http.HandleFunc("/webhooks/recall", a.recallWebhookHandler)
	http.HandleFunc("/demo/recall-sample", a.recallSampleHandler)
	http.HandleFunc("/uploads/presencial/signed-url", a.presencialSignedURLHandler)
	http.HandleFunc("/uploads/presencial/complete", a.presencialUploadCompleteHandler)
	http.HandleFunc("/demo/run", a.demoRunHandler)
	http.HandleFunc("/meetings/", a.getMeetingHandler)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(a.cfg.DataDir))))
}

func (a *App) healthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) recallWebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed reading body", http.StatusBadRequest)
		return
	}
	// PARTE 1 (RECALL): valida assinatura estilo Svix dos webhooks oficiais.
	// Headers esperados: webhook-id, webhook-timestamp, webhook-signature (v1,<base64>).
	if !verifyRecallWebhookSignature(body, r.Header, a.cfg.RecallWebhookSecret) {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	// PARTE 3 (NORMALIZAÇÃO): parseia payload realista da Recall para o contrato interno.
	evt, err := parseRecallWebhookEvent(body)
	if err != nil {
		http.Error(w, "invalid recall webhook payload", http.StatusBadRequest)
		return
	}
	if evt.MeetingID == "" {
		evt.MeetingID = generateMeetingID()
	}
	rawObjectPath, err := a.mediaFactory.PrepareRawFromRecall(r.Context(), evt.MeetingID, evt.MP4URL)
	if err != nil {
		http.Error(w, "failed preparing raw media", http.StatusInternalServerError)
		return
	}
	meeting := &Meeting{
		ID:            evt.MeetingID,
		Source:        SourceOnline,
		Status:        StatusQueued,
		DurationSec:   evt.DurationSeconds,
		RawObjectPath: rawObjectPath,
		RawVideoURL:   evt.MP4URL,
		TranscriptURL: evt.TranscriptURL,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	a.store.Save(meeting)
	a.queue <- MeetingEvent{MeetingID: meeting.ID}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"meeting_id": meeting.ID,
		"status":     meeting.Status,
		"message":    "Webhook recebido e reunião enfileirada para processamento.",
	})
}

func (a *App) presencialSignedURLHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SignedURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.MeetingID == "" {
		req.MeetingID = generateMeetingID()
	}
	if req.Filename == "" {
		req.Filename = req.MeetingID + ".mp4"
	}
	ext := filepath.Ext(req.Filename)
	if ext == "" {
		ext = ".mp4"
	}
	objectPath := filepath.Join("raw", req.MeetingID+ext)
	expiresAt := time.Now().UTC().Add(15 * time.Minute)
	uploadURL := fmt.Sprintf("%s/mock-signed-upload/%s?expires=%d", a.cfg.PublicBaseURL, req.MeetingID, expiresAt.Unix())
	meeting := &Meeting{
		ID:            req.MeetingID,
		Source:        SourcePresencial,
		Status:        StatusWaitingUpload,
		RawObjectPath: objectPath,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	a.store.Save(meeting)
	resp := SignedURLResponse{
		MeetingID:   req.MeetingID,
		ObjectPath:  objectPath,
		UploadURL:   uploadURL,
		ExpiresAt:   expiresAt,
		HowToFinish: "Depois do upload real no bucket, chame POST /uploads/presencial/complete.",
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) presencialUploadCompleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req UploadCompleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	meeting, ok := a.store.Get(req.MeetingID)
	if !ok {
		http.Error(w, "meeting not found", http.StatusNotFound)
		return
	}
	if meeting.Source != SourcePresencial {
		http.Error(w, "meeting is not presencial", http.StatusBadRequest)
		return
	}
	if _, err := a.mediaFactory.PrepareRawFromPresencial(r.Context(), meeting.ID); err != nil {
		http.Error(w, "failed preparing raw media", http.StatusInternalServerError)
		return
	}
	meeting.Status = StatusQueued
	meeting.UpdatedAt = time.Now().UTC()
	a.store.Save(meeting)
	a.queue <- MeetingEvent{MeetingID: meeting.ID}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"meeting_id": meeting.ID,
		"status":     meeting.Status,
		"message":    "Upload confirmado e reunião enfileirada.",
	})
}

func (a *App) demoRunHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req DemoRunRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	source := MeetingSource(strings.ToLower(req.Source))
	if source != SourceOnline && source != SourcePresencial {
		source = SourceOnline
	}
	meetingID := generateMeetingID()
	rawPath := filepath.Join("raw", meetingID+".mp4")
	if source == SourceOnline {
		var err error
		rawPath, err = a.mediaFactory.PrepareRawFromRecall(r.Context(), meetingID, "https://recall.example/mock.mp4")
		if err != nil {
			http.Error(w, "failed preparing online raw", http.StatusInternalServerError)
			return
		}
	} else {
		var err error
		rawPath, err = a.mediaFactory.PrepareRawFromPresencial(r.Context(), meetingID)
		if err != nil {
			http.Error(w, "failed preparing presencial raw", http.StatusInternalServerError)
			return
		}
	}
	meeting := &Meeting{
		ID:            meetingID,
		Source:        source,
		Status:        StatusQueued,
		DurationSec:   180,
		RawObjectPath: rawPath,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	a.store.Save(meeting)
	a.queue <- MeetingEvent{MeetingID: meetingID}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"meeting_id": meetingID,
		"source":     source,
		"status":     meeting.Status,
		"message":    "Pipeline demo iniciado.",
	})
}

func (a *App) recallSampleHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	meetingID := r.URL.Query().Get("meeting_id")
	if strings.TrimSpace(meetingID) == "" {
		meetingID = generateMeetingID()
	}
	durationSec := 3600
	payload := map[string]any{
		"event": "recording.done",
		"data": map[string]any{
			"bot": map[string]any{
				"id": "bot_demo_123",
				"metadata": map[string]any{
					"meeting_id": meetingID,
				},
			},
			"recording": map[string]any{
				"id":       "rec_demo_123",
				"duration": durationSec,
			},
			"media_shortcuts": map[string]any{
				"video_mixed": map[string]any{
					"data": map[string]any{
						"download_url": "https://recall.example/video.mp4",
					},
				},
				"transcript": map[string]any{
					"data": map[string]any{
						"download_url": "https://recall.example/transcript.json",
					},
				},
			},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to build payload", http.StatusInternalServerError)
		return
	}
	id, ts, sig := buildRecallSvixHeaders(body, a.cfg.RecallWebhookSecret)

	endpoint := fmt.Sprintf("%s/webhooks/recall", a.cfg.PublicBaseURL)
	escapedBody := strings.ReplaceAll(string(body), `'`, `'\''`)
	curl := fmt.Sprintf(
		"curl -X POST %s -H 'Content-Type: application/json' -H 'webhook-id: %s' -H 'webhook-timestamp: %s' -H 'webhook-signature: %s' -d '%s'",
		endpoint, id, ts, sig, escapedBody,
	)

	resp := RecallSampleResponse{
		Endpoint: endpoint,
		Method:   "POST",
		Headers: map[string]string{
			"Content-Type":      "application/json",
			"webhook-id":        id,
			"webhook-timestamp": ts,
			"webhook-signature": sig,
		},
		Payload: payload,
		Curl:    curl,
		Notes: []string{
			"Endpoint utilitario para demo local; nao usar em producao.",
			"Assinatura segue o padrao v1,<base64_hmac_sha256> sobre id.timestamp.body.",
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *App) getMeetingHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/meetings/")
	if id == "" {
		http.Error(w, "missing meeting id", http.StatusBadRequest)
		return
	}
	meeting, ok := a.store.Get(id)
	if !ok {
		http.Error(w, "meeting not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, meeting)
}

func (a *App) workerLoop() {
	for evt := range a.queue {
		ctx := context.Background()
		if err := a.processMeeting(ctx, evt.MeetingID); err != nil {
			log.Printf("failed processing meeting=%s err=%v", evt.MeetingID, err)
		}
	}
}

func (a *App) processMeeting(ctx context.Context, meetingID string) error {
	meeting, ok := a.store.Get(meetingID)
	if !ok {
		return fmt.Errorf("meeting not found: %s", meetingID)
	}
	meeting.Status = StatusProcessing
	meeting.UpdatedAt = time.Now().UTC()
	a.store.Save(meeting)

	var transcript []Utterance
	var err error
	switch meeting.Source {
	case SourceOnline:
		transcript, err = a.recallClient.FetchTranscript(ctx, meeting)
	case SourcePresencial:
		transcript, err = a.azureClient.TranscribeWithSpeakerID(ctx, meeting)
	default:
		err = fmt.Errorf("unknown source %s", meeting.Source)
	}
	if err != nil {
		meeting.Status = StatusFailed
		meeting.Error = "transcript failed: " + err.Error()
		meeting.UpdatedAt = time.Now().UTC()
		a.store.Save(meeting)
		return err
	}

	topics, err := a.vertexClient.SegmentTopics(ctx, transcript)
	if err != nil {
		meeting.Status = StatusFailed
		meeting.Error = "topic segmentation failed: " + err.Error()
		meeting.UpdatedAt = time.Now().UTC()
		a.store.Save(meeting)
		return err
	}

	topicsWithAssets, err := a.mediaFactory.GenerateTopicAssets(ctx, meeting.ID, meeting.RawObjectPath, topics)
	if err != nil {
		meeting.Status = StatusFailed
		meeting.Error = "media factory failed: " + err.Error()
		meeting.UpdatedAt = time.Now().UTC()
		a.store.Save(meeting)
		return err
	}

	payload := &FinalPayload{
		MeetingID:   meeting.ID,
		Source:      meeting.Source,
		DurationSec: meeting.DurationSec,
		Transcript:  transcript,
		Topics:      topicsWithAssets,
		GeneratedAt: time.Now().UTC(),
		DemoWarnings: []string{
			"Dados de transcript/tópicos/clips mockados para prova de conceito.",
			"TODOs no código indicam pontos de integração real.",
		},
	}
	if err := a.deliveryClient.Deliver(ctx, payload); err != nil {
		meeting.Status = StatusFailed
		meeting.Error = "delivery failed: " + err.Error()
		meeting.UpdatedAt = time.Now().UTC()
		a.store.Save(meeting)
		return err
	}

	meeting.Transcript = transcript
	meeting.Topics = topicsWithAssets
	meeting.FinalPayload = payload
	meeting.Status = StatusCompleted
	meeting.UpdatedAt = time.Now().UTC()
	a.store.Save(meeting)
	return nil
}

// PARTE 1 (RECALL): assinatura no padrão Svix.
// payload assinado: "<webhook-id>.<webhook-timestamp>.<raw-body>"
// header signature esperado: "v1,<base64-hmac-sha256>"
func verifyRecallWebhookSignature(body []byte, headers http.Header, secret string) bool {
	if secret == "" {
		return false
	}
	webhookID := headers.Get("webhook-id")
	webhookTS := headers.Get("webhook-timestamp")
	webhookSig := headers.Get("webhook-signature")

	// Compatibilidade com demos antigas.
	if webhookID == "" || webhookTS == "" || webhookSig == "" {
		legacy := headers.Get("X-Recall-Signature")
		if legacy == "" {
			return false
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expectedHex := hex.EncodeToString(mac.Sum(nil))
		return hmac.Equal([]byte(expectedHex), []byte(strings.TrimSpace(legacy)))
	}

	toSign := webhookID + "." + webhookTS + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(toSign))
	expectedB64 := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	for _, candidate := range strings.FieldsFunc(webhookSig, func(r rune) bool { return r == ' ' || r == ',' }) {
		if strings.HasPrefix(candidate, "v1") {
			continue
		}
		if hmac.Equal([]byte(candidate), []byte(expectedB64)) {
			return true
		}
	}
	return false
}

func buildRecallSvixHeaders(body []byte, secret string) (webhookID, webhookTS, webhookSignature string) {
	webhookID = "msg_" + generateMeetingID()
	webhookTS = strconv.FormatInt(time.Now().UTC().Unix(), 10)
	toSign := webhookID + "." + webhookTS + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(toSign))
	webhookSignature = "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return webhookID, webhookTS, webhookSignature
}

// PARTE 3 (NORMALIZAÇÃO): parse do webhook Recall (realista) + fallback legado.
func parseRecallWebhookEvent(body []byte) (normalizedRecallIngestion, error) {
	var env recallWebhookEnvelope
	if err := json.Unmarshal(body, &env); err == nil && env.Event != "" {
		out := normalizedRecallIngestion{
			MeetingID:       firstNonEmpty(env.Data.Bot.Metadata.MeetingID, env.Data.Bot.ID, env.Data.Recording.ID),
			MP4URL:          env.Data.MediaShortcuts.Video.Data.DownloadURL,
			TranscriptURL:   env.Data.MediaShortcuts.Transcript.Data.DownloadURL,
			DurationSeconds: int(env.Data.Recording.Duration),
		}
		if out.MeetingID == "" {
			return normalizedRecallIngestion{}, errors.New("missing meeting id in recall payload")
		}
		if out.MP4URL == "" {
			return normalizedRecallIngestion{}, errors.New("missing mp4 download url in recall payload")
		}
		return out, nil
	}

	var legacy legacyRecallWebhookEvent
	if err := json.Unmarshal(body, &legacy); err != nil {
		return normalizedRecallIngestion{}, err
	}
	if legacy.MeetingID == "" && legacy.MP4URL == "" {
		return normalizedRecallIngestion{}, errors.New("unknown recall payload format")
	}
	return normalizedRecallIngestion{
		MeetingID:       legacy.MeetingID,
		MP4URL:          legacy.MP4URL,
		TranscriptURL:   legacy.TranscriptURL,
		DurationSeconds: legacy.DurationSeconds,
	}, nil
}

// PARTE 3 (NORMALIZAÇÃO): Recall utterances -> transcript unificado.
func normalizeRecallTranscript(raw recallTranscriptResponse) []Utterance {
	out := make([]Utterance, 0, len(raw.Utterances))
	for _, u := range raw.Utterances {
		start := int(u.StartSec)
		end := int(u.EndSec)
		if end < start {
			end = start
		}
		speaker := u.Speaker
		if strings.TrimSpace(speaker) == "" {
			speaker = "Unknown"
		}
		out = append(out, Utterance{
			StartSec: start,
			EndSec:   end,
			Speaker:  speaker,
			Text:     strings.TrimSpace(u.Text),
		})
	}
	return out
}

// PARTE 3 (NORMALIZAÇÃO): Azure Fast Transcription -> transcript unificado.
func normalizeAzureFastTranscript(raw azureFastTranscription) []Utterance {
	out := make([]Utterance, 0, len(raw.RecognizedPhrases))
	for _, p := range raw.RecognizedPhrases {
		start := int(p.OffsetMilliseconds / 1000)
		end := int((p.OffsetMilliseconds + p.DurationMilliseconds) / 1000)
		if end < start {
			end = start
		}
		text := ""
		if len(p.NBest) > 0 {
			text = strings.TrimSpace(p.NBest[0].Display)
		}
		out = append(out, Utterance{
			StartSec: start,
			EndSec:   end,
			Speaker:  fmt.Sprintf("Speaker-%02d", p.Speaker),
			Text:     text,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func generateMeetingID() string {
	return fmt.Sprintf("mtg-%d-%04d", time.Now().Unix(), rand.Intn(10000))
}

type geminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *geminiAPIClient) SegmentTopics(ctx context.Context, transcript []Utterance) ([]TopicSegment, error) {
	if len(transcript) == 0 {
		return nil, nil
	}

	var lines []string
	for _, u := range transcript {
		lines = append(lines, fmt.Sprintf("[%02d:%02d-%02d:%02d] %s: %s",
			u.StartSec/60, u.StartSec%60, u.EndSec/60, u.EndSec%60, u.Speaker, u.Text))
	}

	prompt := strings.Join([]string{
		"Voce e um segmentador de reunioes de investimento.",
		"Recebera uma transcricao com timestamps e falantes.",
		"Retorne APENAS JSON valido (sem markdown) no formato:",
		`{"topics":[{"title":"...","start_sec":0,"end_sec":10}]}`,
		"Regras:",
		"- 3 a 8 topicos.",
		"- start_sec e end_sec inteiros.",
		"- sem sobreposicao.",
		"- ordenar por start_sec.",
		"- manter escopo de reuniao de deals.",
		"",
		"Transcricao:",
		strings.Join(lines, "\n"),
	}, "\n")

	reqBody := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": prompt},
				},
			},
		},
		"generationConfig": map[string]any{
			"temperature":      0.2,
			"responseMimeType": "application/json",
		},
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini api status=%d body=%s", res.StatusCode, string(body))
	}

	var gr geminiGenerateResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return nil, fmt.Errorf("invalid gemini response: %w", err)
	}
	if gr.Error != nil {
		return nil, fmt.Errorf("gemini error: %s", gr.Error.Message)
	}
	if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("gemini returned no candidates")
	}

	raw := cleanJSONResponse(gr.Candidates[0].Content.Parts[0].Text)
	var parsed struct {
		Topics []TopicSegment `json:"topics"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("failed parsing topics json: %w | raw=%s", err, raw)
	}
	if len(parsed.Topics) == 0 {
		return nil, fmt.Errorf("gemini returned empty topics")
	}

	for i := range parsed.Topics {
		if parsed.Topics[i].Title == "" {
			parsed.Topics[i].Title = "Topico " + strconv.Itoa(i+1)
		}
	}
	return parsed.Topics, nil
}

func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`(?s)\{.*\}`)
	if m := re.FindString(s); m != "" {
		return m
	}
	return s
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
