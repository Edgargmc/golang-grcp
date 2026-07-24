package main

import (
	"testing"

	pb "grpc-todo/gen"
)

func TestSQLiteRepository_InsertFindSaveDelete(t *testing.T) {
	repo, err := newSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}

	item := &pb.TodoItem{Title: "Probar SQLite", Description: "desc", CreatedAt: "2026-01-01T00:00:00Z"}
	if err := repo.Insert(item); err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if item.Id == 0 {
		t.Fatal("expected Insert to assign a non-zero Id")
	}

	found, ok := repo.FindByID(item.Id)
	if !ok {
		t.Fatal("expected to find the inserted item")
	}
	if found.Title != "Probar SQLite" {
		t.Errorf("expected title %q, got %q", "Probar SQLite", found.Title)
	}

	found.Completed = true
	if err := repo.Save(found); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded, ok := repo.FindByID(item.Id)
	if !ok {
		t.Fatal("expected to find the item after Save")
	}
	if !reloaded.Completed {
		t.Error("expected Completed to be true after Save")
	}

	all := repo.FindAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 item in FindAll, got %d", len(all))
	}

	deleted, ok := repo.Delete(item.Id)
	if !ok {
		t.Fatal("expected Delete to succeed")
	}
	if deleted.Id != item.Id {
		t.Errorf("expected deleted item id %d, got %d", item.Id, deleted.Id)
	}

	if _, ok := repo.FindByID(item.Id); ok {
		t.Error("expected item to be gone after Delete")
	}
}
