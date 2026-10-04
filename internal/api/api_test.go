package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/aeksunone/go-api-test-kit/internal/api"
	"github.com/aeksunone/go-api-test-kit/internal/store"
)

// Every fixture owns its store; tests can run in parallel without sharing data.
func fixture() (http.Handler, *store.Memory) {
	m := store.NewMemory()
	return api.New(m), m
}

func request(h http.Handler, method, path, token, body, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func status(t *testing.T, w *httptest.ResponseRecorder, want int) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, want, w.Body.String())
	}
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q", got)
	}
	var v T
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode response: %v; body = %s", err, w.Body.String())
	}
	return v
}

func errorBody(t *testing.T, w *httptest.ResponseRecorder, wantStatus int, message string) {
	t.Helper()
	status(t, w, wantStatus)
	got := decode[map[string]string](t, w)
	if !reflect.DeepEqual(got, map[string]string{"error": message}) {
		t.Fatalf("error body = %#v, want %q", got, message)
	}
}

func seed(t *testing.T, m *store.Memory, owner int64, title string) store.Task {
	t.Helper()
	v, err := m.Create(context.Background(), owner, title)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestHealthWithoutAuthentication(t *testing.T) {
	t.Parallel()
	h, _ := fixture()
	w := request(h, "GET", "/healthz", "", "", "")
	status(t, w, http.StatusOK)
	if got := decode[map[string]string](t, w); !reflect.DeepEqual(got, map[string]string{"status": "ok"}) {
		t.Fatalf("health = %#v", got)
	}
}

func TestAuthentication(t *testing.T) {
	t.Parallel()
	headers := []struct {
		name   string
		values []string
	}{
		{"missing", nil}, {"empty", []string{""}}, {"wrong scheme", []string{"Basic demo-alice"}},
		{"missing token", []string{"Bearer"}}, {"extra field", []string{"Bearer demo-alice extra"}},
		{"invalid token", []string{"Bearer unknown"}}, {"case sensitive token", []string{"Bearer DEMO-ALICE"}},
		{"duplicate identical", []string{"Bearer demo-alice", "Bearer demo-alice"}},
		{"duplicate different", []string{"Bearer demo-alice", "Bearer demo-bob"}},
		{"comma joined", []string{"Bearer demo-alice, Bearer demo-bob"}},
	}
	endpoints := []struct{ method, path string }{
		{"GET", "/users/me"}, {"GET", "/tasks"}, {"POST", "/tasks"},
		{"GET", "/tasks/1"}, {"PUT", "/tasks/1"}, {"DELETE", "/tasks/1"},
	}
	for _, tc := range headers {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, ep := range endpoints {
				t.Run(ep.method+ep.path, func(t *testing.T) {
					h, m := fixture()
					original := seed(t, m, 1, "untouched")
					r := httptest.NewRequest(ep.method, ep.path, strings.NewReader(`{"title":"changed","done":true}`))
					r.Header.Set("Content-Type", "application/json")
					for _, value := range tc.values {
						r.Header.Add("Authorization", value)
					}
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					errorBody(t, w, 401, "unauthorized")
					if got := w.Header().Get("WWW-Authenticate"); got != `Bearer realm="demo"` {
						t.Fatalf("challenge = %q", got)
					}
					tasks, err := m.List(context.Background(), 1)
					if err != nil || !reflect.DeepEqual(tasks, []store.Task{original}) {
						t.Fatalf("unauthenticated request changed data: %#v, %v", tasks, err)
					}
				})
			}
		})
	}
}

func TestUsersMe(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		token, name string
		id          int64
	}{{"demo-alice", "Alice", 1}, {"demo-bob", "Bob", 2}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, _ := fixture()
			w := request(h, "GET", "/users/me", tc.token, "", "")
			status(t, w, 200)
			got := decode[struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
			}](t, w)
			if got.ID != tc.id || got.Name != tc.name {
				t.Fatalf("identity = %#v", got)
			}
		})
	}
}

func TestBearerSchemeIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	h, _ := fixture()
	r := httptest.NewRequest("GET", "/users/me", nil)
	r.Header.Set("Authorization", "bEaReR demo-alice")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	status(t, w, 200)
}

func TestCRUDLifecycle(t *testing.T) {
	t.Parallel()
	h, _ := fixture()
	w := request(h, "GET", "/tasks", "demo-alice", "", "")
	status(t, w, 200)
	if got := decode[[]store.Task](t, w); got == nil || len(got) != 0 {
		t.Fatalf("initial list = %#v, want empty array", got)
	}
	w = request(h, "POST", "/tasks", "demo-alice", `{"title":"  Learn HTTP tests  "}`, "application/json; charset=utf-8")
	status(t, w, 201)
	created := decode[store.Task](t, w)
	if created.ID < 1 || created.OwnerID != 1 || created.Title != "Learn HTTP tests" || created.Done {
		t.Fatalf("created = %#v", created)
	}
	path := "/tasks/" + strconv.FormatInt(created.ID, 10)
	if got := w.Header().Get("Location"); got != path {
		t.Fatalf("Location = %q, want %q", got, path)
	}
	w = request(h, "GET", path, "demo-alice", "", "")
	status(t, w, 200)
	if got := decode[store.Task](t, w); got != created {
		t.Fatalf("read = %#v, want %#v", got, created)
	}
	w = request(h, "PUT", path, "demo-alice", `{"title":"  Finished  ","done":true}`, "application/json")
	status(t, w, 200)
	updated := decode[store.Task](t, w)
	want := store.Task{ID: created.ID, OwnerID: 1, Title: "Finished", Done: true}
	if updated != want {
		t.Fatalf("updated = %#v, want %#v", updated, want)
	}
	w = request(h, "GET", path, "demo-alice", "", "")
	status(t, w, 200)
	if got := decode[store.Task](t, w); got != want {
		t.Fatalf("persisted update = %#v", got)
	}
	w = request(h, "GET", "/tasks", "demo-alice", "", "")
	status(t, w, 200)
	if got := decode[[]store.Task](t, w); !reflect.DeepEqual(got, []store.Task{want}) {
		t.Fatalf("list = %#v", got)
	}
	w = request(h, "DELETE", path, "demo-alice", "", "")
	status(t, w, 204)
	if w.Body.Len() != 0 {
		t.Fatalf("204 response has body %q", w.Body.String())
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		w = request(h, method, path, "demo-alice", `{"title":"cannot revive"}`, "application/json")
		errorBody(t, w, 404, "task not found")
	}
	w = request(h, "GET", "/tasks", "demo-alice", "", "")
	status(t, w, 200)
	if got := decode[[]store.Task](t, w); got == nil || len(got) != 0 {
		t.Fatalf("final list = %#v", got)
	}
}

func TestCrossOwnerAccessIsHiddenAndDoesNotMutate(t *testing.T) {
	t.Parallel()
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			t.Parallel()
			h, m := fixture()
			original := seed(t, m, 1, "Alice's private task")
			path := fmt.Sprintf("/tasks/%d", original.ID)
			w := request(h, method, path, "demo-bob", `{"title":"Bob tried to change this","done":true}`, "application/json")
			errorBody(t, w, 404, "task not found")
			absent := request(h, method, "/tasks/999999", "demo-bob", `{"title":"valid"}`, "application/json")
			errorBody(t, absent, 404, "task not found")
			if w.Body.String() != absent.Body.String() {
				t.Fatal("foreign and nonexistent IDs have distinguishable errors")
			}
			got, err := m.Get(context.Background(), 1, original.ID)
			if err != nil || got != original {
				t.Fatalf("owner data changed: %#v, %v", got, err)
			}
			w = request(h, "GET", path, "demo-alice", "", "")
			status(t, w, 200)
			if got := decode[store.Task](t, w); got != original {
				t.Fatalf("owner read = %#v", got)
			}
		})
	}
}

func TestListOnlyContainsOwnerTasksInIDOrder(t *testing.T) {
	t.Parallel()
	h, m := fixture()
	alice1 := seed(t, m, 1, "Alice first")
	bob := seed(t, m, 2, "Bob only")
	alice2 := seed(t, m, 1, "Alice second")
	for _, tc := range []struct {
		token string
		want  []store.Task
	}{{"demo-alice", []store.Task{alice1, alice2}}, {"demo-bob", []store.Task{bob}}} {
		t.Run(tc.token, func(t *testing.T) {
			w := request(h, "GET", "/tasks", tc.token, "", "")
			status(t, w, 200)
			if got := decode[[]store.Task](t, w); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("list = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestInputValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, contentType string
		code                    int
		message                 string
	}{
		{"missing content type", `{"title":"valid"}`, "", 415, "content type must be application/json"},
		{"text content type", `{"title":"valid"}`, "text/plain", 415, "content type must be application/json"},
		{"malformed media type", `{"title":"valid"}`, "application/json; broken", 415, "content type must be application/json"},
		{"empty body", "", "application/json", 400, "invalid JSON body"},
		{"malformed JSON", `{"title":`, "application/json", 400, "invalid JSON body"},
		{"array", `[]`, "application/json", 400, "invalid JSON body"},
		{"string", `"hello"`, "application/json", 400, "invalid JSON body"},
		{"unknown field", `{"title":"valid","admin":true}`, "application/json", 400, "invalid JSON body"},
		{"owner injection", `{"title":"valid","owner_id":2}`, "application/json", 400, "invalid JSON body"},
		{"wrong title type", `{"title":123}`, "application/json", 400, "invalid JSON body"},
		{"wrong done type", `{"title":"valid","done":"yes"}`, "application/json", 400, "invalid JSON body"},
		{"two objects", `{"title":"valid"} {"title":"second"}`, "application/json", 400, "body must contain one JSON object"},
		{"trailing garbage", `{"title":"valid"} nope`, "application/json", 400, "body must contain one JSON object"},
		{"null", "null", "application/json", 400, "title must contain 1 to 200 characters"},
		{"missing title", `{}`, "application/json", 400, "title must contain 1 to 200 characters"},
		{"empty title", `{"title":""}`, "application/json", 400, "title must contain 1 to 200 characters"},
		{"blank title", `{"title":" \t\n "}`, "application/json", 400, "title must contain 1 to 200 characters"},
		{"201 runes", `{"title":"` + strings.Repeat("界", 201) + `"}`, "application/json", 400, "title must contain 1 to 200 characters"},
		{"oversize body", `{"title":"` + strings.Repeat("x", 5000) + `"}`, "application/json", 400, "invalid JSON body"},
	}
	for _, method := range []string{"POST", "PUT"} {
		t.Run(method, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					h, m := fixture()
					original := seed(t, m, 1, "untouched")
					path := "/tasks"
					if method == "PUT" {
						path += fmt.Sprintf("/%d", original.ID)
					}
					w := request(h, method, path, "demo-alice", tc.body, tc.contentType)
					errorBody(t, w, tc.code, tc.message)
					got, err := m.List(context.Background(), 1)
					if err != nil || !reflect.DeepEqual(got, []store.Task{original}) {
						t.Fatalf("invalid request changed data: %#v, %v", got, err)
					}
				})
			}
		})
	}
}

func TestTitleBoundariesAndUnicode(t *testing.T) {
	t.Parallel()
	for _, title := range []string{"x", strings.Repeat("x", 200), strings.Repeat("界", 200)} {
		t.Run(fmt.Sprintf("%d_bytes", len(title)), func(t *testing.T) {
			t.Parallel()
			h, _ := fixture()
			body, _ := json.Marshal(map[string]string{"title": title})
			w := request(h, "POST", "/tasks", "demo-bob", string(body)+" \n\t", "application/json")
			status(t, w, 201)
			got := decode[store.Task](t, w)
			if got.Title != title || got.OwnerID != 2 || got.Done {
				t.Fatalf("created = %#v", got)
			}
		})
	}
}

func TestCreateCannotStartDone(t *testing.T) {
	t.Parallel()
	h, m := fixture()
	w := request(h, "POST", "/tasks", "demo-alice", `{"title":"valid","done":true}`, "application/json")
	errorBody(t, w, 400, "new tasks must start incomplete")
	got, _ := m.List(context.Background(), 1)
	if len(got) != 0 {
		t.Fatalf("rejected create persisted: %#v", got)
	}
}

func TestInvalidTaskIDs(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"0", "-1", "abc", "1.2", "9223372036854775808"} {
		t.Run(id, func(t *testing.T) {
			t.Parallel()
			for _, method := range []string{"GET", "PUT", "DELETE"} {
				h, _ := fixture()
				w := request(h, method, "/tasks/"+id, "demo-alice", `{"title":"valid"}`, "application/json")
				errorBody(t, w, 400, "invalid task id")
			}
		})
	}
}

// Failing implementations verify that no database details escape over HTTP.
type failingStore struct{ err error }

func (f failingStore) List(context.Context, int64) ([]store.Task, error) { return nil, f.err }
func (f failingStore) Create(context.Context, int64, string) (store.Task, error) {
	return store.Task{}, f.err
}
func (f failingStore) Get(context.Context, int64, int64) (store.Task, error) {
	return store.Task{}, f.err
}
func (f failingStore) Update(context.Context, int64, int64, string, bool) (store.Task, error) {
	return store.Task{}, f.err
}
func (f failingStore) Delete(context.Context, int64, int64) error { return f.err }

func TestStoreErrors(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		err     error
		code    int
		message string
	}{
		{"internal", errors.New("secret database connection details"), 500, "internal server error"},
		{"wrapped not found", fmt.Errorf("query failed: %w", store.ErrNotFound), 404, "task not found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, ep := range []struct{ method, path string }{{"GET", "/tasks"}, {"POST", "/tasks"}, {"GET", "/tasks/1"}, {"PUT", "/tasks/1"}, {"DELETE", "/tasks/1"}} {
				w := request(api.New(failingStore{tc.err}), ep.method, ep.path, "demo-alice", `{"title":"valid"}`, "application/json")
				errorBody(t, w, tc.code, tc.message)
				if w.Header().Get("Location") != "" {
					t.Fatal("failed request set Location")
				}
			}
		})
	}
}
