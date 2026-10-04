package store_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/aeksunone/go-api-test-kit/internal/store"
)

const (
	aliceID int64 = 1
	bobID   int64 = 2
)

// Every case gets a fresh repository. The same behavior is required of the
// fast in-memory store and the real PostgreSQL implementation.
func repositoryContract(t *testing.T, fresh func(*testing.T) store.Tasks) {
	t.Helper()
	t.Run("CRUD", func(t *testing.T) {
		t.Parallel()
		repo := fresh(t)
		ctx := context.Background()
		created, err := repo.Create(ctx, aliceID, "write tests")
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if created.ID <= 0 || created.OwnerID != aliceID || created.Title != "write tests" || created.Done {
			t.Fatalf("unexpected created task: %+v", created)
		}
		got, err := repo.Get(ctx, aliceID, created.ID)
		if err != nil || got != created {
			t.Fatalf("Get = %+v, %v; want %+v", got, err, created)
		}
		want := created
		want.Title, want.Done = "tests written", true
		updated, err := repo.Update(ctx, aliceID, created.ID, want.Title, want.Done)
		if err != nil || updated != want {
			t.Fatalf("Update = %+v, %v; want %+v", updated, err, want)
		}
		got, err = repo.Get(ctx, aliceID, created.ID)
		if err != nil || got != want {
			t.Fatalf("Get after Update = %+v, %v; want %+v", got, err, want)
		}
		if err := repo.Delete(ctx, aliceID, created.ID); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		_, err = repo.Get(ctx, aliceID, created.ID)
		wantNotFound(t, err)
		wantNotFound(t, repo.Delete(ctx, aliceID, created.ID))
	})

	t.Run("OwnershipHidesExistence", func(t *testing.T) {
		t.Parallel()
		repo := fresh(t)
		ctx := context.Background()
		original, err := repo.Create(ctx, aliceID, "Alice's private task")
		if err != nil {
			t.Fatal(err)
		}
		_, err = repo.Get(ctx, bobID, original.ID)
		wantNotFound(t, err)
		_, err = repo.Update(ctx, bobID, original.ID, "stolen", true)
		wantNotFound(t, err)
		wantNotFound(t, repo.Delete(ctx, bobID, original.ID))
		got, err := repo.Get(ctx, aliceID, original.ID)
		if err != nil || got != original {
			t.Fatalf("foreign-owner operations changed task: got %+v, %v; want %+v", got, err, original)
		}
		assertList(t, repo, bobID, []store.Task{})
	})

	t.Run("MissingTask", func(t *testing.T) {
		t.Parallel()
		repo := fresh(t)
		ctx := context.Background()
		_, err := repo.Get(ctx, aliceID, 999)
		wantNotFound(t, err)
		_, err = repo.Update(ctx, aliceID, 999, "missing", true)
		wantNotFound(t, err)
		wantNotFound(t, repo.Delete(ctx, aliceID, 999))
	})

	t.Run("ListIsOrderedScopedAndNonNil", func(t *testing.T) {
		t.Parallel()
		repo := fresh(t)
		ctx := context.Background()
		assertList(t, repo, aliceID, []store.Task{})
		first, err := repo.Create(ctx, aliceID, "zebra")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.Create(ctx, bobID, "Bob only"); err != nil {
			t.Fatal(err)
		}
		second, err := repo.Create(ctx, aliceID, "apple")
		if err != nil {
			t.Fatal(err)
		}
		if second.ID <= first.ID {
			t.Fatalf("IDs are not increasing: %d then %d", first.ID, second.ID)
		}
		for range 3 {
			assertList(t, repo, aliceID, []store.Task{first, second})
		}
		for _, task := range []store.Task{first, second} {
			if err := repo.Delete(ctx, aliceID, task.ID); err != nil {
				t.Fatal(err)
			}
		}
		assertList(t, repo, aliceID, []store.Task{})
	})
}

func wantNotFound(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("error = %v; want ErrNotFound", err)
	}
}

func assertList(t *testing.T, repo store.Tasks, owner int64, want []store.Task) {
	t.Helper()
	got, err := repo.List(context.Background(), owner)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %#v; want %#v (non-nil)", got, want)
	}
}

func TestMemoryContract(t *testing.T) {
	t.Parallel()
	repositoryContract(t, func(t *testing.T) store.Tasks { return store.NewMemory() })
}

// Concurrent callers share one repository, so -race exercises its locking.
func TestMemoryConcurrentCreate(t *testing.T) {
	t.Parallel()
	repo := store.NewMemory()
	const callers = 32
	type result struct {
		task store.Task
		err  error
	}
	results := make(chan result, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			task, err := repo.Create(context.Background(), aliceID, "concurrent")
			results <- result{task, err}
		}()
	}
	wg.Wait()
	close(results)
	seen := make(map[int64]bool, callers)
	for result := range results {
		if result.err != nil {
			t.Fatalf("concurrent Create: %v", result.err)
		}
		if seen[result.task.ID] {
			t.Fatalf("duplicate ID %d", result.task.ID)
		}
		seen[result.task.ID] = true
	}
	tasks, err := repo.List(context.Background(), aliceID)
	if err != nil || len(tasks) != callers {
		t.Fatalf("List after concurrent creation: %d tasks, %v; want %d", len(tasks), err, callers)
	}
}
