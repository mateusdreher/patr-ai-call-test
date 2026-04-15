package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyRecallWebhookSignature(t *testing.T) {
	body := []byte(`{"event":"recording.done","data":{"bot":{"id":"bot_123"}}}`)
	secret := "demo-secret"
	id := "msg_123"
	ts := "1700000000"

	macSvix := hmac.New(sha256.New, []byte(secret))
	macSvix.Write([]byte(id + "." + ts + "." + string(body)))
	svixSig := "v1," + base64.StdEncoding.EncodeToString(macSvix.Sum(nil))

	headers := make(http.Header)
	headers.Set("webhook-id", id)
	headers.Set("webhook-timestamp", ts)
	headers.Set("webhook-signature", svixSig)
	if !verifyRecallWebhookSignature(body, headers, secret) {
		t.Fatalf("expected valid svix signature")
	}

	legacyHeaders := make(http.Header)
	macLegacy := hmac.New(sha256.New, []byte(secret))
	macLegacy.Write(body)
	legacyHeaders.Set("X-Recall-Signature", hex.EncodeToString(macLegacy.Sum(nil)))
	if !verifyRecallWebhookSignature(body, legacyHeaders, secret) {
		t.Fatalf("expected valid legacy signature")
	}

	headers.Set("webhook-signature", "v1,invalid")
	if verifyRecallWebhookSignature(body, headers, secret) {
		t.Fatalf("expected invalid svix signature to fail")
	}
}

func TestParseRecallWebhookEvent(t *testing.T) {
	body := []byte(`{
		"event":"recording.done",
		"data":{
			"bot":{"id":"bot_1","metadata":{"meeting_id":"mtg-abc"}},
			"recording":{"id":"rec_1","duration":321},
			"media_shortcuts":{
				"video_mixed":{"data":{"download_url":"https://example.com/video.mp4"}},
				"transcript":{"data":{"download_url":"https://example.com/transcript.json"}}
			}
		}
	}`)
	got, err := parseRecallWebhookEvent(body)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if got.MeetingID != "mtg-abc" || got.MP4URL == "" || got.DurationSeconds != 321 {
		t.Fatalf("unexpected normalized recall payload: %+v", got)
	}
}

func TestNormalizeAzureFastTranscript(t *testing.T) {
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
				Speaker:              2,
				OffsetMilliseconds:   5000,
				DurationMilliseconds: 7000,
				NBest: []struct {
					Display string `json:"display"`
				}{{Display: "Texto 1"}},
			},
		},
	}
	out := normalizeAzureFastTranscript(raw)
	if len(out) != 1 {
		t.Fatalf("expected 1 utterance, got %d", len(out))
	}
	if out[0].StartSec != 5 || out[0].EndSec != 12 || out[0].Speaker != "Speaker-02" {
		t.Fatalf("unexpected normalized utterance: %+v", out[0])
	}
}

func TestRecallSampleHandlerBuildsVerifiableHeaders(t *testing.T) {
	app := &App{
		cfg: Config{
			PublicBaseURL:       "http://localhost:8080",
			RecallWebhookSecret: "demo-secret",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/demo/recall-sample?meeting_id=mtg-xyz", nil)
	rr := httptest.NewRecorder()
	app.recallSampleHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rr.Code, rr.Body.String())
	}

	var resp RecallSampleResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	body, err := json.Marshal(resp.Payload)
	if err != nil {
		t.Fatalf("failed to re-marshal payload: %v", err)
	}
	headers := make(http.Header)
	for k, v := range resp.Headers {
		headers.Set(k, v)
	}
	if !verifyRecallWebhookSignature(body, headers, app.cfg.RecallWebhookSecret) {
		t.Fatalf("expected util headers to verify")
	}
}

func TestProcessMeetingOnline(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		cfg:            Config{PublicBaseURL: "http://localhost:8080", DataDir: dir},
		store:          NewStore(),
		queue:          make(chan MeetingEvent, 1),
		recallReview:   NewRecallReviewService(Config{PublicBaseURL: "http://localhost:8080", DataDir: dir}),
		recallClient:   &mockRecallClient{},
		azureClient:    &mockAzureClient{},
		vertexClient:   &mockVertexClient{},
		mediaFactory:   &mockMediaFactory{baseDir: dir, publicBaseURL: "http://localhost:8080"},
		deliveryClient: &mockDeliveryClient{},
	}
	meeting := &Meeting{
		ID:            "mtg-test-1",
		Source:        SourceOnline,
		Status:        StatusQueued,
		DurationSec:   180,
		RawObjectPath: filepath.Join("raw", "mtg-test-1.mp4"),
	}
	app.store.Save(meeting)

	if err := app.processMeeting(context.Background(), meeting.ID); err != nil {
		t.Fatalf("processMeeting returned error: %v", err)
	}
	got, ok := app.store.Get(meeting.ID)
	if !ok {
		t.Fatalf("meeting not found")
	}
	if got.Status != StatusCompleted {
		t.Fatalf("expected status completed, got %s", got.Status)
	}
	if len(got.Topics) == 0 {
		t.Fatalf("expected topics generated")
	}
	if got.FinalPayload == nil {
		t.Fatalf("expected final payload generated")
	}
}

func TestRecallReviewLifecycle(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		cfg:          Config{PublicBaseURL: "http://localhost:8080", DataDir: dir},
		recallReview: NewRecallReviewService(Config{PublicBaseURL: "http://localhost:8080", DataDir: dir}),
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/review/recall/bots", strings.NewReader(`{
		"bot_name":"Lifecycle Bot",
		"meeting_url":"https://meet.google.com/lifecycle-demo",
		"recording_mode":"speaker_view",
		"transcription":"real_time"
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRR := httptest.NewRecorder()
	app.recallBotsHandler(createRR, createReq)
	if createRR.Code != http.StatusCreated {
		t.Fatalf("unexpected create status=%d body=%s", createRR.Code, createRR.Body.String())
	}

	var created RecallReviewBot
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("failed to parse bot: %v", err)
	}

	startRR := httptest.NewRecorder()
	startReq := httptest.NewRequest(http.MethodPost, "/api/review/recall/bots/"+created.ID+"/start_recording", nil)
	app.recallBotByIDHandler(startRR, startReq)
	if startRR.Code != http.StatusCreated {
		t.Fatalf("unexpected start status=%d body=%s", startRR.Code, startRR.Body.String())
	}

	var recording RecallReviewRecording
	if err := json.Unmarshal(startRR.Body.Bytes(), &recording); err != nil {
		t.Fatalf("failed to parse recording: %v", err)
	}

	stopRR := httptest.NewRecorder()
	stopReq := httptest.NewRequest(http.MethodPost, "/api/review/recall/bots/"+created.ID+"/stop_recording", nil)
	app.recallBotByIDHandler(stopRR, stopReq)
	if stopRR.Code != http.StatusOK {
		t.Fatalf("unexpected stop status=%d body=%s", stopRR.Code, stopRR.Body.String())
	}

	getRR := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/api/review/recall/recordings/"+recording.ID, nil)
	app.recallRecordingByIDHandler(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("unexpected retrieve status=%d body=%s", getRR.Code, getRR.Body.String())
	}

	var stopped RecallReviewRecording
	if err := json.Unmarshal(getRR.Body.Bytes(), &stopped); err != nil {
		t.Fatalf("failed to parse stopped recording: %v", err)
	}
	if stopped.Status != "completed" {
		t.Fatalf("expected completed recording, got %s", stopped.Status)
	}
	if stopped.MediaShortcuts["transcript"].ID == "" || stopped.MediaShortcuts["video_mixed"].ID == "" {
		t.Fatalf("expected media shortcuts generated: %+v", stopped.MediaShortcuts)
	}
}

func TestRecallCatalogHandler(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		cfg:          Config{PublicBaseURL: "http://localhost:8080", DataDir: dir},
		recallReview: NewRecallReviewService(Config{PublicBaseURL: "http://localhost:8080", DataDir: dir}),
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/review/recall/catalog", nil)
	app.recallCatalogHandler(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("unexpected status=%d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Endpoints []RecallEndpointDoc `json:"endpoints"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse catalog: %v", err)
	}
	if len(resp.Endpoints) < 10 {
		t.Fatalf("expected broad recall catalog, got %d endpoints", len(resp.Endpoints))
	}
}
