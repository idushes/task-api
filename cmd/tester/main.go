package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	WorkerA = "worker_a"
	WorkerB = "worker_b"
)

type Config struct {
	PostgresURL string
	RabbitMQURL string
	APIUrl      string
}

func main() {
	cfg := Config{
		PostgresURL: os.Getenv("POSTGRES_URL"),
		RabbitMQURL: os.Getenv("RABBITMQ_URL"),
		APIUrl:      "http://127.0.0.1:8080",
	}

	if cfg.PostgresURL == "" || cfg.RabbitMQURL == "" {
		log.Fatal("POSTGRES_URL and RABBITMQ_URL must be set")
	}

	// 1. Clean DB
	cleanDB(cfg.PostgresURL)

	// 2. Setup RabbitMQ consumer
	msgsA, closeA := consumeQueue(cfg.RabbitMQURL, WorkerA)
	defer closeA()
	msgsB, closeB := consumeQueue(cfg.RabbitMQURL, WorkerB)
	defer closeB()

	// Test 1: Simple Task Flow
	log.Println(">>> Starting Test 1: Simple Task Flow")
	taskID := createTask(cfg.APIUrl, WorkerA, nil, map[string]interface{}{"msg": "hello"})
	log.Printf("Created task %s", taskID)

	// Verify message in Queue A
	verifyMessage(msgsA, taskID)

	// Complete task
	completeTask(cfg.APIUrl, taskID, map[string]interface{}{"status": "done"})
	log.Println("Task completed. Test 1 Passed.")

	// Test 2: Tree Flow (Parent waiting for children)
	log.Println("\n>>> Starting Test 2: Tree Flow")
	// Create Parent (Worker A)
	parentID := createTask(cfg.APIUrl, WorkerA, nil, map[string]interface{}{"role": "parent"})
	log.Printf("Created Parent %s", parentID)
	// Consuming parent creation message to clear queue
	verifyMessage(msgsA, parentID)

	// Create Child 1 (Worker B)
	child1ID := createTask(cfg.APIUrl, WorkerB, &parentID, map[string]interface{}{"role": "child1"})
	log.Printf("Created Child1 %s", child1ID)
	verifyMessage(msgsB, child1ID)

	// Create Child 2 (Worker B)
	child2ID := createTask(cfg.APIUrl, WorkerB, &parentID, map[string]interface{}{"role": "child2"})
	log.Printf("Created Child2 %s", child2ID)
	verifyMessage(msgsB, child2ID)

	// Complete Child 1
	completeTask(cfg.APIUrl, child1ID, map[string]interface{}{"res": 1})
	log.Println("Completed Child 1. Verifying NO message for Parent yet...")

	// Ensure NO message in Queue A (Test Wait)
	select {
	case msg := <-msgsA:
		log.Fatalf("Unexpected message in Queue A: %s", msg.Body)
	case <-time.After(500 * time.Millisecond):
		log.Println("No message in Queue A as expected.")
	}

	// Complete Child 2
	completeTask(cfg.APIUrl, child2ID, map[string]interface{}{"res": 2})
	log.Println("Completed Child 2. Expecting Parent message in Queue A...")

	// Verify Parent Message in Queue A with Subtasks
	msg := verifyMessage(msgsA, parentID)
	// Check content for subtasks
	var body map[string]interface{}
	json.Unmarshal(msg.Body, &body)
	payload := body["payload"].(map[string]interface{})
	subtasks := payload["subtasks"].([]interface{})
	if len(subtasks) != 2 {
		log.Fatalf("Expected 2 subtasks results, got %d", len(subtasks))
	}
	log.Println("Parent received with subtasks! Test 2 Passed.")

	// Optional: Complete the Parent task to leave DB in clean state
	completeTask(cfg.APIUrl, parentID, map[string]interface{}{"status": "parent_done"})
	log.Println("Parent task completed.")

	// Test 3: Duplicate Completion
	log.Println("\n>>> Starting Test 3: Duplicate Completion")
	err := completeTaskExpectError(cfg.APIUrl, child1ID, map[string]interface{}{"res": 1}, 409)
	if err != nil {
		log.Fatalf("Test 3 Failed: %v", err)
	}
	log.Println("Got expected conflict error. Test 3 Passed.")

	// Test 4: Invalid Parent ID
	log.Println("\n>>> Starting Test 4: Invalid Parent ID")
	randomParentID := "00000000-0000-0000-0000-000000000000"
	createTaskExpectError(cfg.APIUrl, WorkerA, &randomParentID, map[string]interface{}{"msg": "orphan"}, 500)
	log.Println("Got expected error for missing parent. Test 4 Passed.")

	// Test 5: Deep Tree (Grandchild -> Child -> Parent)
	log.Println("\n>>> Starting Test 5: Deep Tree")
	// Level 1: Parent
	rootID := createTask(cfg.APIUrl, WorkerA, nil, map[string]interface{}{"level": 1})
	verifyMessage(msgsA, rootID)
	// Level 2: Child
	midID := createTask(cfg.APIUrl, WorkerB, &rootID, map[string]interface{}{"level": 2})
	verifyMessage(msgsB, midID)
	// Level 3: Grandchild
	leafID := createTask(cfg.APIUrl, WorkerB, &midID, map[string]interface{}{"level": 3})
	verifyMessage(msgsB, leafID)

	// Complete Leaf -> Should trigger Mid?
	// Wait, Mid is just a TASK. Does it have "Incomplete Children"?
	// Yes, `midID` is the parent of `leafID`.
	// Only when `leafID` completes, we check `midID`.
	// If `midID` has NO other children, we trigger `midID`.
	// BUT `midID` itself must be incomplete? Yes.
	// Does trigger mean "Publish midID to Queue"? Yes.

	completeTask(cfg.APIUrl, leafID, map[string]interface{}{"val": "leaf_done"})

	// Expect message for midID in Queue B (since mid's worker is WorkerB)
	// Verification: The message for midID should contain the result from leafID in "subtasks".
	msgMid := verifyMessage(msgsB, midID)
	// Check subtasks
	var bodyMid map[string]interface{}
	json.Unmarshal(msgMid.Body, &bodyMid)
	payloadMid := bodyMid["payload"].(map[string]interface{})
	if _, ok := payloadMid["subtasks"]; !ok {
		log.Fatalf("Expected subtasks in Middle Node")
	}
	log.Println("Middle node triggered by Leaf completion.")

	// Now complete Middle Node (it was just triggered)
	// It's processed by worker... worker sends result.
	completeTask(cfg.APIUrl, midID, map[string]interface{}{"val": "mid_done"})

	// Expect message for rootID in Queue A
	msgRoot := verifyMessage(msgsA, rootID)
	var bodyRoot map[string]interface{}
	json.Unmarshal(msgRoot.Body, &bodyRoot)
	payloadRoot := bodyRoot["payload"].(map[string]interface{})
	subtasksRoot := payloadRoot["subtasks"].([]interface{})
	// subtasksRoot should contain result of midID
	if len(subtasksRoot) != 1 {
		log.Fatalf("Expected 1 subtask for Root")
	}
	log.Println("Root node triggered by Middle completion. Test 5 Passed.")

	// Optional: Complete Root task
	completeTask(cfg.APIUrl, rootID, map[string]interface{}{"val": "root_done"})
	log.Println("Root task completed.")

	log.Println("\nALL TESTS PASSED!")
}

func cleanDB(url string) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec("TRUNCATE TABLE tasks CASCADE")
	if err != nil {
		// If table doesn't exist, try applying schema
		log.Println("Table not found, applying schema...")
		schema, err := os.ReadFile("migrations/schema.sql")
		if err != nil {
			log.Fatalf("Failed to read schema: %v", err)
		}
		if _, err := db.Exec(string(schema)); err != nil {
			log.Fatalf("Failed to apply schema: %v", err)
		}
	} else {
		log.Println("Database cleaned.")
	}
}

func consumeQueue(url, qName string) (<-chan amqp.Delivery, func()) {
	conn, err := amqp.Dial(url)
	if err != nil {
		log.Fatal(err)
	}
	ch, err := conn.Channel()
	if err != nil {
		log.Fatal(err)
	}
	_, err = ch.QueueDeclare(qName, true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}
	msgs, err := ch.Consume(qName, "", true, false, false, false, nil)
	if err != nil {
		log.Fatal(err)
	}
	return msgs, func() { ch.Close(); conn.Close() }
}

func createTask(url, worker string, parentID *string, payload interface{}) string {
	body := map[string]interface{}{
		"payload": payload,
	}
	if parentID != nil {
		body["parent_id"] = *parentID
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(url+"/task/"+worker, "application/json", bytes.NewBuffer(b))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("CreateTask failed: %s %s", resp.Status, string(b))
	}
	var res map[string]string
	json.NewDecoder(resp.Body).Decode(&res)
	return res["id"]
}

func createTaskExpectError(url, worker string, parentID *string, payload interface{}, expectedStatus int) {
	body := map[string]interface{}{
		"payload": payload,
	}
	if parentID != nil {
		body["parent_id"] = *parentID
	}
	b, _ := json.Marshal(body)
	resp, err := http.Post(url+"/task/"+worker, "application/json", bytes.NewBuffer(b))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("Expected status %d, got %d. Body: %s", expectedStatus, resp.StatusCode, string(b))
	}
}

func completeTask(url, id string, result interface{}) {
	body, _ := json.Marshal(map[string]interface{}{"result": result})
	resp, err := http.Post(url+"/task/"+id, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("CompleteTask failed: %s %s", resp.Status, string(b))
	}
}

func completeTaskExpectError(url, id string, result interface{}, expectedStatus int) error {
	body, _ := json.Marshal(map[string]interface{}{"result": result})
	resp, err := http.Post(url+"/task/"+id, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("Expected status %d, got %d", expectedStatus, resp.StatusCode)
	}
	return nil
}

func verifyMessage(msgs <-chan amqp.Delivery, expectedID string) amqp.Delivery {
	select {
	case msg := <-msgs:
		var body map[string]interface{}
		if err := json.Unmarshal(msg.Body, &body); err != nil {
			log.Fatalf("Failed to parse msg: %v", err)
		}
		if body["id"] != expectedID {
			log.Fatalf("Expected msg ID %s, got %v", expectedID, body["id"])
		}
		return msg
	case <-time.After(2 * time.Second):
		log.Fatalf("Timeout waiting for message %s", expectedID)
		return amqp.Delivery{}
	}
}
