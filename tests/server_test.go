package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	. "gograph"
)

var (
	demoTypesOnce sync.Once
	demoTypesErr  error
)

func phase3RunRequest(t *testing.T) []byte {
	t.Helper()

	request := RunRequest{
		Graph: GraphSpec{
			Entry: "__start__",
			Nodes: []NodeSpec{
				{ID: "dispatch", Type: "dispatch", Config: map[string]any{}},
				{ID: "Race Car1", Type: "race_car", Config: map[string]any{"name": "Race Car1"}},
				{ID: "Race Car2", Type: "race_car", Config: map[string]any{"name": "Race Car2"}},
				{ID: "Race Car3", Type: "race_car", Config: map[string]any{"name": "Race Car3"}},
				{ID: "join", Type: "join", Config: map[string]any{}},
			},
			Edges: []EdgeSpec{
				{From: START, To: "dispatch"},
				{From: "dispatch", To: "Race Car1"},
				{From: "dispatch", To: "Race Car2"},
				{From: "dispatch", To: "Race Car3"},
				{From: "Race Car1", To: "join"},
				{From: "Race Car2", To: "join"},
				{From: "Race Car3", To: "join"},
				{From: "join", To: END},
			},
			Channels: []ChannelSpec{
				{Key: "messages", Reducer: "append"},
				{Key: "done", Reducer: "add"},
			},
		},
		Initial: State{Data: map[string]any{}},
	}

	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func ensureDemoTypes(t *testing.T) {
	t.Helper()
	demoTypesOnce.Do(func() {
		demoTypesErr = RegisterDemoTypes()
	})
	if demoTypesErr != nil {
		t.Fatal(demoTypesErr)
	}
}

func TestValidateEndpointSuccess(t *testing.T) {
	ensureDemoTypes(t)
	body := phase3RunRequest(t)
	var graphRequest RunRequest
	if err := json.Unmarshal(body, &graphRequest); err != nil {
		t.Fatal(err)
	}

	graphJSON, err := json.Marshal(graphRequest.Graph)
	if err != nil {
		t.Fatal(err)
	}
	httpRequest := httptest.NewRequest(http.MethodPost, "/graph/validate", bytes.NewReader(graphJSON))
	recorder := httptest.NewRecorder()

	NewServer().ServeHTTP(recorder, httpRequest)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["valid"] != true {
		t.Fatalf("valid = %v, want true", response["valid"])
	}
}

func TestRegisteredNodeTypesEndpoint(t *testing.T) {
	ensureDemoTypes(t)
	request := httptest.NewRequest(http.MethodGet, "/registry/nodes", nil)
	recorder := httptest.NewRecorder()

	NewServer().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response struct {
		Types []string `json:"types"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	registered := make(map[string]bool, len(response.Types))
	for _, nodeType := range response.Types {
		registered[nodeType] = true
	}
	for _, nodeType := range []string{"dispatch", "join", "race_car"} {
		if !registered[nodeType] {
			t.Errorf("types = %v, missing %q", response.Types, nodeType)
		}
	}
}

func TestValidateEndpointFailure(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/graph/validate", strings.NewReader(`{"entry":"__start__","nodes":[],"edges":[]}`))
	recorder := httptest.NewRecorder()

	NewServer().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}

	var response map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["valid"] != false {
		t.Fatalf("valid = %v, want false", response["valid"])
	}
	if response["error"] == "" {
		t.Fatal("expected validation error")
	}
}

func TestRunEndpointReturnsDoneCount(t *testing.T) {
	ensureDemoTypes(t)
	request := httptest.NewRequest(http.MethodPost, "/graph/run", bytes.NewReader(phase3RunRequest(t)))
	recorder := httptest.NewRecorder()

	NewServer().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var state State
	if err := json.Unmarshal(recorder.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.Data["done"] != float64(3) {
		t.Fatalf("done = %v, want 3", state.Data["done"])
	}
}

func TestStreamEndpointEmitsThreeSSEEvents(t *testing.T) {
	ensureDemoTypes(t)
	request := httptest.NewRequest(http.MethodPost, "/graph/stream", bytes.NewReader(phase3RunRequest(t)))
	recorder := httptest.NewRecorder()

	NewServer().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", contentType)
	}

	body, err := io.ReadAll(recorder.Result().Body)
	if err != nil {
		t.Fatal(err)
	}
	output := string(body)
	if count := strings.Count(output, "event: step\n"); count != 3 {
		t.Fatalf("step event count = %d, want 3; body = %s", count, output)
	}
	for _, active := range []string{"dispatch", "Race Car1", "Race Car2", "Race Car3", "join"} {
		if !strings.Contains(output, active) {
			t.Errorf("stream does not contain %q; body = %s", active, output)
		}
	}
}

func TestCORSPreflightEndpoints(t *testing.T) {
	for _, path := range []string{"/graph/validate", "/graph/run", "/graph/stream"} {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodOptions, path, nil)
			request.Header.Set("Origin", "http://localhost:5173")
			recorder := httptest.NewRecorder()

			NewServer().ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
			}
			if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
				t.Errorf("allow-origin = %q", got)
			}
			if got := recorder.Header().Get("Access-Control-Allow-Methods"); got != "POST, OPTIONS" {
				t.Errorf("allow-methods = %q", got)
			}
			if got := recorder.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, Accept" {
				t.Errorf("allow-headers = %q", got)
			}
		})
	}
}
