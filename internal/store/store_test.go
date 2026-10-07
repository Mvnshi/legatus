package store

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Mvnshi/legatus/internal/model"
)

func TestSaveLoadListAndMissing(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	older := &model.Run{ID: "aaaa1111", Status: model.Queued, CreatedAt: time.Now().Add(-time.Hour)}
	newer := &model.Run{ID: "bbbb2222", Status: model.Running, CreatedAt: time.Now(), Steps: []model.StepState{{ID: "implement", Kind: model.StepAgent}}}
	for _, r := range []*model.Run{older, newer} {
		if err := s.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Load("bbbb2222")
	if err != nil || got.Status != model.Running || len(got.Steps) != 1 || got.UpdatedAt.IsZero() {
		t.Fatalf("Load = %+v, %v", got, err)
	}
	list, err := s.List()
	if err != nil || len(list) != 2 || list[0].ID != "bbbb2222" {
		t.Fatalf("List = %v, %v (want newest first)", list, err)
	}
	if _, err := s.Load("cccc3333"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing run: %v", err)
	}
}

func TestRunIDsCannotEscapeTheStore(t *testing.T) {
	s, _ := Open(t.TempDir())
	for _, id := range []string{"../x", "a/b", `a\b`, "", "A", ".."} {
		if _, err := s.Load(id); err == nil || errors.Is(err, ErrNotFound) {
			t.Errorf("id %q was accepted: %v", id, err)
		}
		if err := s.Save(&model.Run{ID: id}); err == nil {
			t.Errorf("Save accepted id %q", id)
		}
	}
}

func TestJournalAppendsAndResumesFromALine(t *testing.T) {
	s, _ := Open(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := s.Append(model.Event{Run: "aaaa1111", Type: "tick", Data: map[string]any{"i": i}}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	all, next, err := s.Events("aaaa1111", 0)
	if err != nil || len(all) != 20 || next != 20 {
		t.Fatalf("Events = %d events, next %d, %v", len(all), next, err)
	}
	rest, next, _ := s.Events("aaaa1111", 15)
	if len(rest) != 5 || next != 20 {
		t.Fatalf("from 15: %d events, next %d", len(rest), next)
	}
	none, _, err := s.Events("dddd4444", 0)
	if err != nil || len(none) != 0 {
		t.Fatalf("journal of an unknown run: %v, %v", none, err)
	}
}
