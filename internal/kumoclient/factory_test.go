package kumoclient

import (
	"errors"
	"testing"

	"github.com/kumobase/kumo-cli/internal/config"
)

func TestNewRequiresAPIKey(t *testing.T) {
	_, err := New(config.Settings{Profile: "default", BaseURL: config.DefaultBaseURL})
	if !errors.Is(err, ErrNotLoggedIn) {
		t.Fatalf("expected ErrNotLoggedIn, got %v", err)
	}
}

func TestNewSucceedsWithAPIKey(t *testing.T) {
	c, err := New(config.Settings{
		Profile: "default",
		BaseURL: config.DefaultBaseURL,
		APIKey:  "kumo_sk_test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewRejectsBadBaseURL(t *testing.T) {
	// An empty base URL is rejected by the SDK constructor.
	_, err := New(config.Settings{Profile: "default", BaseURL: "", APIKey: "kumo_sk_test"})
	if err == nil {
		t.Fatal("expected error for empty base URL")
	}
}
