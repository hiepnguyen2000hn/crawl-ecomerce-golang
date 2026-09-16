package rabbitmq

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Config holds the connection and topology settings shared by
// Publisher and Consumer.
type Config struct {
	URL      string
	Exchange string
}

func dlqName(queue, suffix string) string {
	return queue + suffix
}

// DeclareTopology declares a topic exchange, a queue bound to routingKey,
// and a dead-letter queue that receives messages the main queue rejects
// or lets expire.
func DeclareTopology(ch *amqp.Channel, exchange, queue, routingKey, dlqSuffix string) error {
	if err := ch.ExchangeDeclare(exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: declare exchange: %w", err)
	}

	dlq := dlqName(queue, dlqSuffix)
	if _, err := ch.QueueDeclare(dlq, true, false, false, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: declare dlq: %w", err)
	}
	if err := ch.QueueBind(dlq, dlq, exchange, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: bind dlq: %w", err)
	}

	args := amqp.Table{
		"x-dead-letter-exchange":    exchange,
		"x-dead-letter-routing-key": dlq,
	}
	if _, err := ch.QueueDeclare(queue, true, false, false, false, args); err != nil {
		return fmt.Errorf("rabbitmq: declare queue: %w", err)
	}
	if err := ch.QueueBind(queue, routingKey, exchange, false, nil); err != nil {
		return fmt.Errorf("rabbitmq: bind queue: %w", err)
	}
	return nil
}

// Publisher publishes messages to a topic exchange.
type Publisher struct {
	cfg  Config
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewPublisher(cfg Config) (*Publisher, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: channel: %w", err)
	}
	if err := ch.ExchangeDeclare(cfg.Exchange, "topic", true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: declare exchange: %w", err)
	}
	return &Publisher{cfg: cfg, conn: conn, ch: ch}, nil
}

func (p *Publisher) Publish(ctx context.Context, routingKey string, body []byte) error {
	return p.ch.PublishWithContext(ctx, p.cfg.Exchange, routingKey, false, false, amqp.Publishing{
		ContentType: "application/json",
		Body:        body,
	})
}

func (p *Publisher) Close() error {
	if err := p.ch.Close(); err != nil {
		return err
	}
	return p.conn.Close()
}

// Consumer consumes messages from a named queue.
type Consumer struct {
	cfg  Config
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewConsumer(cfg Config) (*Consumer, error) {
	conn, err := amqp.Dial(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq: dial: %w", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq: channel: %w", err)
	}
	return &Consumer{cfg: cfg, conn: conn, ch: ch}, nil
}

// Consume declares the queue's topology, then blocks handling deliveries
// until ctx is cancelled. A handler error nacks the delivery without
// requeue, sending it toward the dead-letter queue.
func (c *Consumer) Consume(ctx context.Context, queue, routingKey string, handler func([]byte) error) error {
	if err := DeclareTopology(c.ch, c.cfg.Exchange, queue, routingKey, ".dlq"); err != nil {
		return err
	}
	msgs, err := c.ch.Consume(queue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("rabbitmq: consume: %w", err)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case d, ok := <-msgs:
			if !ok {
				return fmt.Errorf("rabbitmq: delivery channel closed")
			}
			if err := handler(d.Body); err != nil {
				_ = d.Nack(false, false)
				continue
			}
			_ = d.Ack(false)
		}
	}
}

func (c *Consumer) Close() error {
	if err := c.ch.Close(); err != nil {
		return err
	}
	return c.conn.Close()
}
