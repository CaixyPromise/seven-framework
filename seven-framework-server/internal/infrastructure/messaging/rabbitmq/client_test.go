package rabbitmq

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	msgoutbox "github.com/CaixyPromise/seven-framework/seven-framework-server/internal/infrastructure/messaging/outbox"
	amqp "github.com/rabbitmq/amqp091-go"
)

func TestWaitForPublishOutcome(t *testing.T) {
	tests := []struct {
		name      string
		populate  func(chan amqp.Confirmation, chan amqp.Return)
		wantError error
	}{
		{
			name: "acknowledged",
			populate: func(confirms chan amqp.Confirmation, _ chan amqp.Return) {
				confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
			},
		},
		{
			name: "negative acknowledgment",
			populate: func(confirms chan amqp.Confirmation, _ chan amqp.Return) {
				confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: false}
			},
			wantError: ErrPublishNack,
		},
		{
			name: "mandatory return",
			populate: func(_ chan amqp.Confirmation, returns chan amqp.Return) {
				returns <- amqp.Return{ReplyCode: 312, ReplyText: "NO_ROUTE", Exchange: "events", RoutingKey: "missing"}
			},
			wantError: ErrPublishUnroutable,
		},
		{
			name: "returned message wins over acknowledgment",
			populate: func(confirms chan amqp.Confirmation, returns chan amqp.Return) {
				returns <- amqp.Return{ReplyCode: 312, ReplyText: "NO_ROUTE"}
				confirms <- amqp.Confirmation{DeliveryTag: 1, Ack: true}
			},
			wantError: ErrPublishUnroutable,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirms := make(chan amqp.Confirmation, 1)
			returns := make(chan amqp.Return, 1)
			tt.populate(confirms, returns)
			err := waitForPublishOutcome(context.Background(), confirms, returns)
			if !errors.Is(err, tt.wantError) {
				t.Fatalf("waitForPublishOutcome() error=%v, want %v", err, tt.wantError)
			}
		})
	}
}

func TestWaitForPublishOutcomeFailsWhenConfirmChannelCloses(t *testing.T) {
	confirms := make(chan amqp.Confirmation)
	close(confirms)
	if err := waitForPublishOutcome(context.Background(), confirms, make(chan amqp.Return)); !errors.Is(err, ErrPublishConfirmLost) {
		t.Fatalf("waitForPublishOutcome() error=%v, want %v", err, ErrPublishConfirmLost)
	}
}

func TestConsumeRetryDispositionRetriesUnclassifiedFailuresWithoutHotLooping(t *testing.T) {
	requeue, delay := consumeRetryDisposition(msgoutbox.ConsumeLeaseHeldError{})
	if !requeue || delay != time.Second {
		t.Fatalf("held consume lease disposition=(%t, %s), want (true, %s)", requeue, delay, time.Second)
	}

	requeue, delay = consumeRetryDisposition(errors.New("database temporarily unavailable"))
	if !requeue || delay != time.Second {
		t.Fatalf("unclassified infrastructure error disposition=(%t, %s), want (true, %s)", requeue, delay, time.Second)
	}
}

func TestConsumeRetryDispositionDeadLettersOnlyExplicitPermanentFailures(t *testing.T) {
	requeue, delay := consumeRetryDisposition(PermanentConsumeError(errors.New("invalid payload")))
	if requeue || delay != 0 {
		t.Fatalf("permanent error disposition=(%t, %s), want (false, 0)", requeue, delay)
	}
}

// Existing RabbitMQ consumers own the repository's ordinary JSON wire
// contract. DG5 has a stricter Sonic envelope, but it must stay in the DG5
// adapter instead of changing notifications/files through this shared client.
func TestGenericRabbitClientKeepsStandardJSONWireContract(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve RabbitMQ client test source")
	}
	payload, err := os.ReadFile(filepath.Join(filepath.Dir(file), "client.go"))
	if err != nil {
		t.Fatalf("read RabbitMQ client source: %v", err)
	}
	source := string(payload)
	if !strings.Contains(source, "encoding/json") {
		t.Fatal("generic RabbitMQ client must use the standard JSON wire codec")
	}
	for _, forbidden := range []string{"bytedance/sonic", "NormalizeForJSON", "QueueDelete", "QueuePurge"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("generic RabbitMQ client leaked DG5 Sonic behavior: %s", forbidden)
		}
	}
}

func TestRabbitClientDoesNotShareTopologyAndConsumerChannels(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve RabbitMQ client test source")
	}
	payload, err := os.ReadFile(filepath.Join(filepath.Dir(file), "client.go"))
	if err != nil {
		t.Fatalf("read RabbitMQ client source: %v", err)
	}
	source := string(payload)
	if strings.Contains(source, "channelSnapshot()") {
		t.Fatal("topology and consumers must not share a client-wide AMQP channel")
	}
	for _, want := range []string{
		"func (c *Client) topologyChannel()",
		"func (c *Client) consumerChannel()",
		"defer closeConsumerChannel(ch)",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("RabbitMQ channel ownership contract missing %q", want)
		}
	}
}

func TestGenericRabbitPayloadUsesStandardJSONSemantics(t *testing.T) {
	type ordinaryPayload struct {
		ID    int64  `json:"id"`
		Title string `json:"title"`
	}
	body, err := marshalGenericJSON(ordinaryPayload{ID: 9007199254740991, Title: "ordinary"})
	if err != nil {
		t.Fatalf("marshal ordinary RabbitMQ payload: %v", err)
	}
	var decoded map[string]any
	if err := stdjson.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decode ordinary JSON payload: %v", err)
	}
	if decoded["title"] != "ordinary" || string(body) != `{"id":9007199254740991,"title":"ordinary"}` {
		t.Fatalf("generic RabbitMQ JSON payload drifted: %s", body)
	}
}

// DG5's raw envelope is a required, confirm-backed protocol. A consumer
// reconnect may close the shared client after the adapter's availability check;
// that second publish-side check must surface an error instead of making the
// relay mark an unbroadcast event DONE.
func TestPublishRawRejectsClosedClient(t *testing.T) {
	client := &Client{enabled: true}
	if err := client.Close(); err != nil {
		t.Fatalf("close test RabbitMQ client: %v", err)
	}
	if err := client.PublishRaw(context.Background(), RawPublishOptions{Body: []byte(`{}`)}); !errors.Is(err, ErrPublisherUnavailable) {
		t.Fatalf("required raw publish after close err=%v, want unavailable", err)
	}
}
