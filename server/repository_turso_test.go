package main

import (
	"os"
	"testing"

	pb "grpc-todo/gen"
)

// Este test solo corre si TURSO_DATABASE_URL / TURSO_AUTH_TOKEN están
// seteadas (nunca hardcodeadas acá). En CI, sin esas variables, se
// saltea solo — nuestra suite principal sigue usando inMemoryRepository.
func TestTursoRepository_InsertFindSaveDelete(t *testing.T) {
	dbURL := os.Getenv("TURSO_DATABASE_URL")
	authToken := os.Getenv("TURSO_AUTH_TOKEN")
	if dbURL == "" || authToken == "" {
		t.Skip("TURSO_DATABASE_URL / TURSO_AUTH_TOKEN no seteados, salteando test contra Turso real")
	}

	repo, err := newTursoRepository(dbURL, authToken)
	if err != nil {
		t.Fatalf("failed to connect to turso: %v", err)
	}

	item := &pb.TodoItem{Title: "Test Turso", Description: "desc", CreatedAt: "2026-01-01T00:00:00Z"}
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
	if found.Title != "Test Turso" {
		t.Errorf("expected title %q, got %q", "Test Turso", found.Title)
	}

	found.Completed = true
	if err := repo.Save(found); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	reloaded, ok := repo.FindByID(item.Id)
	if !ok || !reloaded.Completed {
		t.Fatal("expected Completed to be true after Save")
	}

	if _, ok := repo.Delete(item.Id); !ok {
		t.Fatal("expected Delete to succeed")
	}
}
