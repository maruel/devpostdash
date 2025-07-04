package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockDevpostClient implements devpostClientInterface for testing.
type mockDevpostClient struct{}

func (m *mockDevpostClient) fetchProjects(ctx context.Context, eventID string) ([]*Project, error) {
	if eventID == "fake-event" {
		return []*Project{
			{
				ID:        "1",
				ShortName: "project-one",
				Title:     "Fake Project One",
				URL:       "http://example.com/project-one",
				Tagline:   "This is the first fake project.",
				Image:     "http://example.com/image-one.png",
				Winner:    false,
				Team:      []Person{{Name: "Alice", URL: "http://example.com/alice", AvatarURL: "http://example.com/alice.png"}},
				Likes:     10,
			},
			{
				ID:        "2",
				ShortName: "project-two",
				Title:     "Fake Project Two",
				URL:       "http://example.com/project-two",
				Tagline:   "This is the second fake project.",
				Image:     "http://example.com/image-two.png",
				Winner:    true,
				Team:      []Person{{Name: "Bob", URL: "http://example.com/bob", AvatarURL: "http://example.com/bob.png"}},
				Likes:     20,
			},
		}, nil
	}
	return nil, nil
}

func (m *mockDevpostClient) fetchProject(ctx context.Context, p *Project) error {
	// No-op for this test
	return nil
}

func TestHandleEventCards(t *testing.T) {
	mockClient := &mockDevpostClient{}
	handler := newWebServerHandler(mockClient, nil) // Pass nil for roaster as it's not used in this test

	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/event/fake-event/cards")
	if err != nil {
		t.Fatalf("Failed to make GET request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Fake Project One") {
		t.Errorf("Response body does not contain 'Fake Project One'")
	}
	if !strings.Contains(bodyStr, "Fake Project Two") {
		t.Errorf("Response body does not contain 'Fake Project Two'")
	}
}
