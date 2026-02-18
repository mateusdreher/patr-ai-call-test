package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"
)

func TestVerifyHMACSHA256(t *testing.T) {
	body := []byte(`{"meeting_id":"abc"}`)
	secret := "demo-secret"
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))
	if !verifyHMACSHA256(body, sig, secret) {
		t.Fatalf("expected valid signature")
	}
	if verifyHMACSHA256(body, "invalid", secret) {
		t.Fatalf("expected invalid signature to fail")
	}
}

func TestProcessMeetingOnline(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		cfg:            Config{PublicBaseURL: "http://localhost:8080", DataDir: dir},
		store:          NewStore(),
		queue:          make(chan MeetingEvent, 1),
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
