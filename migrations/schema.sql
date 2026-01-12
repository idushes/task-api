CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS workers (
    name VARCHAR(255) PRIMARY KEY
);

INSERT INTO workers (name) VALUES ('worker_a'), ('worker_b') ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    parent_id UUID REFERENCES tasks(id) ON DELETE CASCADE,
    worker VARCHAR(255) NOT NULL REFERENCES workers(name),
    payload JSONB,
    result JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    is_completed BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_worker_is_completed ON tasks(worker, is_completed);
