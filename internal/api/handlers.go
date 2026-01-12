package api

import (
	"encoding/json"
	"log"
	"net/http"
	"task-api/internal/queue"
	"task-api/internal/storage"

	"github.com/gorilla/mux"
	// Added missing import
	// "github.com/gorilla/mux" // Standard net/http is fine, or we use mux. Mux is better for vars.
	// Actually user didn't specify framework. I'll stick to standard net/http with simple pattern matching if possible,
	// or just use common sense. "GET /task/{id}" implies path params.
	// I'll assume I can use manually parsing or regex, OR slightly better: standard library in Go 1.22 supports wildcards!
	// But let's check go.mod... it said go 1.something. I'll strictly use a simple mux or just split path.
	// I'll grab gorilla/mux to be safe and cleaner.
)

// Add gorilla/mux to deps later if needed, but for now I will implement simple path parsing or just assume standard ServeMux with Go 1.22 features?
// To result in less friction, I will do a quick check if I can use standard lib.
// Wait, I will use `http.ServeMux` and manual parsing to avoid external deps if I can, OR just use `gorilla/mux` which is standard.
// Let's add gorilla/mux to go.mod in a separate step or just use it.
// I'll use it in imports, I'll run `go get` for it.

type Handler struct {
	store *storage.Storage
	queue *queue.Queue
}

func NewHandler(store *storage.Storage, queue *queue.Queue) *Handler {
	return &Handler{
		store: store,
		queue: queue,
	}
}

func (h *Handler) RegisterRoutes(r *mux.Router) {
	// Match UUID for ID-based routes
	r.HandleFunc("/task/{id:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}}", h.CompleteTask).Methods("POST")
	// Match remaining as worker_name
	r.HandleFunc("/task/{worker_name}", h.CreateTask).Methods("POST")
}

// CreateTaskRequest
type CreateTaskRequest struct {
	ParentID *string         `json:"parent_id,omitempty"`
	Payload  json.RawMessage `json:"payload"`
}

func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	workerName := vars["worker_name"]
	if workerName == "" {
		http.Error(w, "Worker name is required", http.StatusBadRequest)
		return
	}

	exists, err := h.store.ValidateWorker(workerName)
	if err != nil {
		log.Printf("Error validating worker: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, "Worker does not exist", http.StatusBadRequest)
		return
	}

	var req CreateTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	task := &storage.Task{
		ParentID: req.ParentID,
		Worker:   workerName,
		Payload:  req.Payload,
	}

	id, err := h.store.CreateTask(task)
	if err != nil {
		log.Printf("Error creating task: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := h.queue.PublishTask(workerName, id, req.Payload); err != nil {
		log.Printf("Error publishing to queue: %v", err)
		// Note: We might want to rollback DB here, but for now keep it simple.
		// Retrying or eventual consistency handled by separate process usually.
		http.Error(w, "Task created but failed to queue", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// CompleteTaskRequest
type CompleteTaskRequest struct {
	Result json.RawMessage `json:"result"`
}

func (h *Handler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var req CompleteTaskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid body", http.StatusBadRequest)
		return
	}

	// 1. Mark task as completed
	err := h.store.CompleteTask(id, req.Result)
	if err != nil {
		if err == storage.ErrTaskAlreadyCompleted {
			http.Error(w, "Task already completed", http.StatusConflict) // User requested error on duplicate
			return
		}
		log.Printf("Error completing task: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// 2. Check parent logic
	t, err := h.store.GetTask(id)
	if err != nil {
		log.Printf("Error fetching task: %v", err)
		return // Task completed, just couldn't fetch to check parent
	}

	if t.ParentID != nil {
		parentID := *t.ParentID

		// Check if any siblings are pending
		count, err := h.store.GetIncompleteChildCount(parentID)
		if err != nil {
			log.Printf("Error checking siblings: %v", err)
			return
		}

		if count == 0 {
			// All children done!
			// 1. Fetch all results
			results, err := h.store.GetChildrenResults(parentID)
			if err != nil {
				log.Printf("Error getting child results: %v", err)
				return
			}

			// 2. Fetch Parent to get its queue/worker (Wait, user said: "радетял отправляем в очередь задач")
			// "радетял" -> "родителя" (parent).
			// We need to fetch the parent task to know where to send it?
			// User said: "родителя отправляем в очередь задач, а результаты прикрепляем как subtasks массив json"
			// Wait, if the parent task was waiting, does it have a worker? Created parent logic usually implies it has a worker.
			// Let's fetch parent.
			parent, err := h.store.GetTask(parentID)
			if err != nil {
				log.Printf("Error fetching parent: %v", err)
				return
			}

			// Construct payload with subtasks results
			// We might want to MERGE with original payload or just valid results?
			// User said: "результаты прикрепляем как subtasks массив json"
			// I'll create a new map for the message body.

			// Warning: We are modifying the payload sent to the worker, NOT the DB payload probably?
			// Or are we supposed to update the parent execution?
			// Usually "re-queueing parent" means it is now ready to process.

			// subtasksJson, _ := json.Marshal(results) // Unused

			// We send `{"id": parentID, "subtasks": [...]}` to the worker?
			// My `PublishTask` wrapper wraps in `{"id":..., "payload":...}`.
			// So I should probably pass a Combined Payload.

			combinedPayload := map[string]interface{}{}
			if len(parent.Payload) > 0 {
				json.Unmarshal(parent.Payload, &combinedPayload)
			}
			var resultObj []interface{}
			for _, r := range results {
				var rObj interface{}
				json.Unmarshal(r, &rObj)
				resultObj = append(resultObj, rObj)
			}
			combinedPayload["subtasks"] = resultObj

			finalPayload, _ := json.Marshal(combinedPayload)

			if err := h.queue.PublishTask(parent.Worker, parent.ID, finalPayload); err != nil {
				log.Printf("Error publishing parent task: %v", err)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
