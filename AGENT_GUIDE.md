# Task API Guide for AI Agents (Antigravity)

This documentation provides the necessary protocol for AI agents (workers) to interact with the Task API system.

## System Overview

The Task API manages asynchronous, hierarchical tasks. Workers listen to RabbitMQ queues for jobs, process them, and report results back to the API. The system handles task state tracking and automatic aggregation of subtask results.

## Protocol Summary

1. **Consume**: Listen to your specific RabbitMQ queue.
2. **Process**: Execute the task logic.
3. **Delegate (Optional)**: Create subtasks via HTTP API.
4. **Report**: Send results to the HTTP API.

---

## 1. RabbitMQ Interface (Input)

Workers must consume messages from a RabbitMQ queue.

- **Queue Name**: `[worker_name]` (e.g., `image_processor`, `email_sender`)
  > **Note**: The `worker_name` must be pre-registered in the system database (`workers` table) for the API to accept tasks for it.
- **Message Format (JSON)**:
  ```json
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "payload": {
      "some_input": "value",
      "subtasks": [...] // Present only if this is a re-queued parent task
    }
  }
  ```
  - `id`: The unique Task ID. Save this for the completion call.
  - `payload`: The input data for the job.

---

## 2. HTTP API Interface (Output)

Workers interact with the API to manage task lifecycle.

**Base URL**: `http://localhost:8080` (Adjust based on environment)

### A. Complete Task (Mandatory)

Call this endpoint when work is finished.

- **Endpoint**: `POST /task/{id}`
- **Param**: `{id}` is the Task ID received in the RabbitMQ message.
- **Header**: `Content-Type: application/json`
- **Body**:
  ```json
  {
    "result": {
      "status": "success",
      "generated_data": "..."
    }
  }
  ```
  - `result`: Arbitrary JSON object representing the work output.

### B. Create Subtask (Optional)

Use this to delegate work to other workers.

- **Endpoint**: `POST /task/{target_worker_name}`
- **Param**: `{target_worker_name}` is the queue name of the desired worker.
- **Body**:
  ```json
  {
    "parent_id": "current_task_id",
    "payload": {
      "input_for_child": "..."
    }
  }
  ```
  - `parent_id`: **Vital**. Pass the ID of the task you are currently processing. This links the tasks.
- **Response**: `201 Created`
  ```json
  { "id": "new_child_task_id" }
  ```

---

## 3. Aggregation Pattern (Subtasks)

The system supports a Map-Reduce style flow.

1. **Phase 1 (Dispatch)**:

   - Your worker receives a task.
   - You determine subtasks are needed.
   - Call **Create Subtask** API multiple times.
   - Call **Complete Task** API (likely with a status like `"waiting_for_children"`).

2. **Phase 2 (Aggregation)**:
   - The system tracks the children.
   - **Once ALL children complete**, the system **Re-queues** the original parent task to your queue.
   - **New Payload**:
     ```json
     {
       "id": "original_task_id",
       "payload": {
         // ... original payload fields ...
         "subtasks": [
           {
             "id": "child_id_1",
             "worker": "worker_a",
             "child_result_1": "some_value",
             "subtasks": []
           },
           {
             "id": "child_id_2",
             "worker": "worker_b",
             "child_result_2": "has_nested_child",
             "subtasks": [
               {
                 "id": "grandchild_id_1",
                 "worker": "worker_c",
                 "grandchild_result": "...",
                 "subtasks": []
               }
             ]
           }
         ]
       }
     }
     ```
   - Your worker receives this new message, sees the `subtasks` field, processes the aggregate results, and calls **Complete Task** again with the final result.

### Data Structure Note

- **Merged Fields**: The system **merges** the `id`, `worker`, and `subtasks` fields directly into your result object (if it is a JSON object). They are NOT wrapped in a separate container.
- **Recursive Subtasks**: The `subtasks` field is a list of results from child tasks. Since each child task can itself have subtasks, this structure is **recursive**. Each item in the `subtasks` array will also contain its own `subtasks: []` field (empty if leaf).
