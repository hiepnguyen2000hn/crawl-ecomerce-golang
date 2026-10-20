// Command dlq-replay moves messages out of a dead-letter queue and back onto
// the exchange, so work that failed for a reason that has since been fixed
// gets another run.
//
// It exists because a dead-letter queue is where messages go to be forgotten:
// RabbitMQ offers no way to replay one, and until an operator moves them back
// the jobs inside are simply lost. The usual case is an Apify account that
// ran out of budget mid-cycle -- every job it touched was dead-lettered even
// though a token with credit was sitting right there.
//
// Messages are taken one at a time and acked only after the republish is
// confirmed, so an interrupted run leaves the rest of the queue untouched.
//
//	go run ./scripts/dlq-replay -dlq fbads.jobs.dlq -routing-key fbads.jobs
//	go run ./scripts/dlq-replay -dlq fbads.jobs.dlq -routing-key fbads.jobs -limit 10
//	go run ./scripts/dlq-replay -dlq fbads.jobs.dlq -routing-key fbads.jobs -dry-run
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	url := flag.String("url", envOr("RABBITMQ_URL", "amqp://guest:guest@localhost:5673/"), "RabbitMQ connection URL")
	exchange := flag.String("exchange", envOr("RABBITMQ_EXCHANGE", "crawl"), "topic exchange to republish to")
	dlq := flag.String("dlq", "", "dead-letter queue to drain, e.g. fbads.jobs.dlq")
	routingKey := flag.String("routing-key", "", "routing key to republish under, e.g. fbads.jobs")
	limit := flag.Int("limit", 0, "stop after this many messages (0 means drain the queue)")
	dryRun := flag.Bool("dry-run", false, "report what would be replayed without moving anything")
	flag.Parse()

	if *dlq == "" || *routingKey == "" {
		fmt.Fprintln(os.Stderr, "both -dlq and -routing-key are required")
		flag.Usage()
		os.Exit(2)
	}

	conn, err := amqp.Dial(*url)
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("channel: %v", err)
	}
	defer ch.Close()

	// Publisher confirms are the whole safety story here: without them a
	// republish that the broker never accepted would still be acked off the
	// dead-letter queue, losing the message for good.
	if err := ch.Confirm(false); err != nil {
		log.Fatalf("put channel into confirm mode: %v", err)
	}

	replayed := 0
	for *limit == 0 || replayed < *limit {
		msg, ok, err := ch.Get(*dlq, false)
		if err != nil {
			log.Fatalf("get from %s: %v", *dlq, err)
		}
		if !ok {
			break
		}

		if *dryRun {
			// Left unacked on purpose. Nacking it back would hand the same
			// message to the next Get and spin forever; holding it instead
			// walks the queue once, and closing the channel on the way out
			// returns every one of them untouched.
			log.Printf("would replay to %s: %s", *routingKey, truncate(msg.Body, 200))
			replayed++
			continue
		}

		confirm, err := ch.PublishWithDeferredConfirm(*exchange, *routingKey, false, false, amqp.Publishing{
			ContentType: msg.ContentType,
			Body:        msg.Body,
		})
		if err != nil {
			_ = msg.Nack(false, true)
			log.Fatalf("republish to %s: %v", *routingKey, err)
		}
		if !confirm.Wait() {
			_ = msg.Nack(false, true)
			log.Fatalf("broker rejected the republish to %s; nothing further moved", *routingKey)
		}
		if err := msg.Ack(false); err != nil {
			// The message is already back on the main queue at this point, so
			// stopping here risks one duplicate rather than a silent loop.
			log.Fatalf("ack after republish (message may be replayed twice): %v", err)
		}
		replayed++
	}

	verb := "replayed"
	if *dryRun {
		verb = "would replay"
	}
	log.Printf("%s %d message(s) from %s to %s", verb, replayed, *dlq, *routingKey)
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
