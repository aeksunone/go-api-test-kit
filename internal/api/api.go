// Package api provides a deliberately small HTTP API for testing examples.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/aeksunone/go-api-test-kit/internal/store"
)

// DemoTokens are public, static demonstration credentials. Replace this entire
// authentication mechanism with a verified identity provider before real use.
var DemoTokens = map[string]int64{"demo-alice": 1, "demo-bob": 2}

type server struct {
	tasks  store.Tasks
	tokens map[string]int64
}

func New(tasks store.Tasks) http.Handler {
	tokens := make(map[string]int64, len(DemoTokens))
	for k, v := range DemoTokens {
		tokens[k] = v
	}
	s := &server{tasks: tasks, tokens: tokens}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /users/me", s.auth(s.me))
	mux.HandleFunc("GET /tasks", s.auth(s.list))
	mux.HandleFunc("POST /tasks", s.auth(s.create))
	mux.HandleFunc("GET /tasks/{id}", s.auth(s.get))
	mux.HandleFunc("PUT /tasks/{id}", s.auth(s.update))
	mux.HandleFunc("DELETE /tasks/{id}", s.auth(s.delete))
	return mux
}
func (s *server) auth(next func(http.ResponseWriter, *http.Request, int64)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			unauthorized(w)
			return
		}
		parts := strings.Fields(values[0])
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			unauthorized(w)
			return
		}
		owner, ok := s.tokens[parts[1]]
		if !ok {
			unauthorized(w)
			return
		}
		next(w, r, owner)
	}
}
func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="demo"`)
	failure(w, 401, "unauthorized")
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func failure(w http.ResponseWriter, status int, message string) {
	respond(w, status, map[string]string{"error": message})
}
func storeError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrNotFound) {
		failure(w, 404, "task not found")
	} else {
		failure(w, 500, "internal server error")
	}
}
func (s *server) me(w http.ResponseWriter, r *http.Request, owner int64) {
	name := "Alice"
	if owner == 2 {
		name = "Bob"
	}
	respond(w, 200, map[string]any{"id": owner, "name": name})
}
func (s *server) list(w http.ResponseWriter, r *http.Request, owner int64) {
	tasks, err := s.tasks.List(r.Context(), owner)
	if err != nil {
		storeError(w, err)
		return
	}
	respond(w, 200, tasks)
}
func id(w http.ResponseWriter, r *http.Request) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || v < 1 {
		failure(w, 400, "invalid task id")
		return 0, false
	}
	return v, true
}

type taskInput struct {
	Title string `json:"title"`
	Done  bool   `json:"done"`
}

func input(w http.ResponseWriter, r *http.Request) (taskInput, bool) {
	var in taskInput
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		failure(w, 415, "content type must be application/json")
		return in, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&in); err != nil {
		failure(w, 400, "invalid JSON body")
		return in, false
	}
	if err := d.Decode(new(any)); err != io.EOF {
		failure(w, 400, "body must contain one JSON object")
		return in, false
	}
	in.Title = strings.TrimSpace(in.Title)
	if utf8.RuneCountInString(in.Title) < 1 || utf8.RuneCountInString(in.Title) > 200 {
		failure(w, 400, "title must contain 1 to 200 characters")
		return in, false
	}
	return in, true
}
func (s *server) create(w http.ResponseWriter, r *http.Request, owner int64) {
	in, ok := input(w, r)
	if !ok {
		return
	}
	if in.Done {
		failure(w, 400, "new tasks must start incomplete")
		return
	}
	task, err := s.tasks.Create(r.Context(), owner, in.Title)
	if err != nil {
		storeError(w, err)
		return
	}
	w.Header().Set("Location", "/tasks/"+strconv.FormatInt(task.ID, 10))
	respond(w, 201, task)
}
func (s *server) get(w http.ResponseWriter, r *http.Request, owner int64) {
	v, ok := id(w, r)
	if !ok {
		return
	}
	t, err := s.tasks.Get(r.Context(), owner, v)
	if err != nil {
		storeError(w, err)
		return
	}
	respond(w, 200, t)
}
func (s *server) update(w http.ResponseWriter, r *http.Request, owner int64) {
	v, ok := id(w, r)
	if !ok {
		return
	}
	in, ok := input(w, r)
	if !ok {
		return
	}
	t, err := s.tasks.Update(r.Context(), owner, v, in.Title, in.Done)
	if err != nil {
		storeError(w, err)
		return
	}
	respond(w, 200, t)
}
func (s *server) delete(w http.ResponseWriter, r *http.Request, owner int64) {
	v, ok := id(w, r)
	if !ok {
		return
	}
	if err := s.tasks.Delete(r.Context(), owner, v); err != nil {
		storeError(w, err)
		return
	}
	w.WriteHeader(204)
}
