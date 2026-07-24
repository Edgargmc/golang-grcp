package main

import (
	"database/sql"

	_ "github.com/tursodatabase/libsql-client-go/libsql" // registra el driver "libsql"
	_ "modernc.org/sqlite"                               // registra el driver "sqlite" para database/sql

	pb "grpc-todo/gen"
)

const schema = `
CREATE TABLE IF NOT EXISTS todos (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	title       TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	completed   INTEGER NOT NULL DEFAULT 0,
	created_at  TEXT NOT NULL
);
`

// sqliteRepository implementa la misma interfaz TodoRepository que
// inMemoryRepository, pero persistiendo en un archivo SQLite real.
type sqliteRepository struct {
	db *sql.DB
}

func newSQLiteRepository(dataSourceName string) (*sqliteRepository, error) {
	db, err := sql.Open("sqlite", dataSourceName)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return &sqliteRepository{db: db}, nil
}

// newTursoRepository conecta a una base remota de Turso en vez de un
// archivo local. Reusa el mismo struct y los mismos métodos que
// newSQLiteRepository — el dialecto SQL es el mismo, solo cambia el
// driver y el destino de la conexión.
func newTursoRepository(dbURL, authToken string) (*sqliteRepository, error) {
	dsn := dbURL + "?authToken=" + authToken

	db, err := sql.Open("libsql", dsn)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}

	return &sqliteRepository{db: db}, nil
}

func (r *sqliteRepository) Insert(item *pb.TodoItem) error {
	res, err := r.db.Exec(
		`INSERT INTO todos (title, description, completed, created_at) VALUES (?, ?, ?, ?)`,
		item.Title, item.Description, item.Completed, item.CreatedAt,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	item.Id = int32(id)

	return nil
}

func (r *sqliteRepository) FindByID(id int32) (*pb.TodoItem, bool) {
	row := r.db.QueryRow(
		`SELECT id, title, description, completed, created_at FROM todos WHERE id = ?`, id,
	)

	item, err := scanTodo(row)
	if err != nil {
		return nil, false
	}

	return item, true
}

func (r *sqliteRepository) FindAll() []*pb.TodoItem {
	rows, err := r.db.Query(`SELECT id, title, description, completed, created_at FROM todos`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var items []*pb.TodoItem
	for rows.Next() {
		item, err := scanTodo(rows)
		if err != nil {
			continue
		}
		items = append(items, item)
	}

	return items
}

func (r *sqliteRepository) Save(item *pb.TodoItem) error {
	_, err := r.db.Exec(
		`UPDATE todos SET title = ?, description = ?, completed = ? WHERE id = ?`,
		item.Title, item.Description, item.Completed, item.Id,
	)
	return err
}

func (r *sqliteRepository) Delete(id int32) (*pb.TodoItem, bool) {
	item, ok := r.FindByID(id)
	if !ok {
		return nil, false
	}

	if _, err := r.db.Exec(`DELETE FROM todos WHERE id = ?`, id); err != nil {
		return nil, false
	}

	return item, true
}

// scanner cubre tanto *sql.Row como *sql.Rows: ambos tienen Scan(...) con
// la misma firma, así que un solo helper sirve para FindByID y FindAll.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanTodo(s scanner) (*pb.TodoItem, error) {
	var item pb.TodoItem
	var completed int

	if err := s.Scan(&item.Id, &item.Title, &item.Description, &completed, &item.CreatedAt); err != nil {
		return nil, err
	}
	item.Completed = completed != 0

	return &item, nil
}
