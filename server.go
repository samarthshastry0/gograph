package gograph

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type RunRequest struct {
	Graph   GraphSpec `json:"graph"`
	Initial State     `json:"initial"`
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(
			"Access-Control-Allow-Origin",
			"http://localhost:5173",
		)
		w.Header().Set(
			"Access-Control-Allow-Methods",
			"POST, OPTIONS",
		)
		w.Header().Set(
			"Access-Control-Allow-Headers",
			"Content-Type, Accept",
		)

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func NewServer() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/registry/nodes", handleRegisteredNodeTypes)
	mux.HandleFunc("/graph/validate", handleValidate)
	mux.HandleFunc("/graph/run", handleRun)
	mux.HandleFunc("/graph/stream", handleStream)
	mux.Handle("/", http.FileServer(http.Dir("web/dist")))

	return withCORS(mux)
}

func handleRegisteredNodeTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method not allowed",
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"types": RegisteredNodeTypes(),
	})
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method not allowed",
		})
		return
	}

	var spec GraphSpec
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&spec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"valid": false,
			"error": "invalid graph JSON: " + err.Error(),
		})
		return
	}

	graph, err := BuildGraph(spec)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	if err := graph.Compile(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"valid": false,
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"valid": true,
	})
}

func handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method not allowed",
		})
		return
	}

	var request RunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid run request JSON: " + err.Error(),
		})
		return
	}

	graph, err := BuildGraph(request.Graph)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	finalState, err := graph.Run(r.Context(), request.Initial)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, finalState)
}

func handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error": "method not allowed",
		})
		return
	}

	var request RunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.UseNumber()

	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": "invalid run request JSON: " + err.Error(),
		})
		return
	}

	graph, err := BuildGraph(request.Graph)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": err.Error(),
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "streaming is not supported",
		})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	_, err = graph.execute(r.Context(), request.Initial, func(event StepEvent) error {
		payload, err := json.Marshal(event)
		if err != nil {
			return err
		}

		if _, err := fmt.Fprintf(w, "event: step\ndata: %s\n\n", payload); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	})

	if err != nil && r.Context().Err() == nil {
		_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", mustJSON(map[string]string{
			"error": err.Error(),
		}))
		flusher.Flush()
	}
}

func mustJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		return []byte(`{"error":"failed to encode error"}`)
	}
	return payload
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
