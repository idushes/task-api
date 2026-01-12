package queue

import (
	"context"
	"encoding/json"
	"log"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Queue struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func New(url string) (*Queue, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, err
	}

	return &Queue{
		conn: conn,
		ch:   ch,
	}, nil
}

func (q *Queue) Close() {
	if q.ch != nil {
		q.ch.Close()
	}
	if q.conn != nil {
		q.conn.Close()
	}
}

func (q *Queue) PublishTask(queueName string, taskID string, payload json.RawMessage) error {
	// Ensure queue exists
	_, err := q.ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return err
	}

	body, err := json.Marshal(map[string]interface{}{
		"id":      taskID,
		"payload": payload,
	})
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = q.ch.PublishWithContext(ctx,
		"",        // exchange
		queueName, // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
	if err != nil {
		log.Printf("Failed to publish message: %v", err)
		return err
	}
	log.Printf("Published task %s to queue %s", taskID, queueName)
	return nil
}
