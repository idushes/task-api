package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"

	_ "github.com/lib/pq"
)

type Task struct {
	ID          string          `json:"id"`
	ParentID    *string         `json:"parent_id,omitempty"`
	Worker      string          `json:"worker"`
	Payload     json.RawMessage `json:"payload"`
	Result      json.RawMessage `json:"result,omitempty"`
	IsCompleted bool            `json:"is_completed"`
}

type Storage struct {
	db *sql.DB
}

func New(postgresURL string) (*Storage, error) {
	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &Storage{db: db}, nil
}

func (s *Storage) InitDB(schemaPath string) error {
	// For simplicity, we assume schema is managed externally or via a simple exec.
	// But here I'll just leave it as we are using schema.sql manually or via tests.
	return nil
}

func (s *Storage) Ping() error {
	return s.db.Ping()
}

func (s *Storage) CreateTask(task *Task) (string, error) {
	var id string
	query := `
		INSERT INTO tasks (parent_id, worker, payload)
		VALUES ($1, $2, $3)
		RETURNING id
	`
	err := s.db.QueryRow(query, task.ParentID, task.Worker, task.Payload).Scan(&id)
	if err != nil {
		return "", err
	}
	return id, nil
}

func (s *Storage) GetTask(id string) (*Task, error) {
	query := `SELECT id, parent_id, worker, payload, result, is_completed FROM tasks WHERE id = $1`
	row := s.db.QueryRow(query, id)

	t := &Task{}
	var parentID sql.NullString
	var result []byte

	err := row.Scan(&t.ID, &parentID, &t.Worker, &t.Payload, &result, &t.IsCompleted)
	if err != nil {
		return nil, err
	}

	if parentID.Valid {
		t.ParentID = &parentID.String
	}
	if result != nil {
		t.Result = result
	}

	return t, nil
}

var ErrTaskAlreadyCompleted = errors.New("task already completed")

func (s *Storage) CompleteTask(id string, result json.RawMessage) error {
	// Check if already completed to prevent double submission
	// We can do this in the UPDATE with a WHERE clause and checking affected rows,
	// or separate check. Affected rows is safer for concurrency.
	query := `UPDATE tasks SET result = $1, is_completed = TRUE WHERE id = $2 AND is_completed = FALSE`
	res, err := s.db.Exec(query, result, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		// Check if it exists but is completed
		t, err := s.GetTask(id)
		if err != nil {
			return err // or ErrNotFound
		}
		if t.IsCompleted {
			return ErrTaskAlreadyCompleted
		}
		return errors.New("task not found")
	}
	return nil
}

func (s *Storage) GetIncompleteChildCount(parentID string) (int, error) {
	query := `SELECT COUNT(*) FROM tasks WHERE parent_id = $1 AND is_completed = FALSE`
	var count int
	err := s.db.QueryRow(query, parentID).Scan(&count)
	return count, err
}

func (s *Storage) GetChildrenResults(parentID string) ([]json.RawMessage, error) {
	query := `SELECT result FROM tasks WHERE parent_id = $1`
	rows, err := s.db.Query(query, parentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []json.RawMessage
	for rows.Next() {
		var r []byte
		if err := rows.Scan(&r); err != nil {
			log.Printf("Failed to scan child result: %v", err)
			continue
		}
		results = append(results, json.RawMessage(r))
	}
	return results, nil
}

func (s *Storage) ValidateWorker(name string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM workers WHERE name = $1)`
	err := s.db.QueryRow(query, name).Scan(&exists)
	return exists, err
}
