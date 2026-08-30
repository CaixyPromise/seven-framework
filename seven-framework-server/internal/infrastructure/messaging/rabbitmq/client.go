package rabbitmq

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/shared/config"
	amqp "github.com/rabbitmq/amqp091-go"
)

var (
	ErrPublishUnroutable    = errors.New("rabbitmq publish was returned as unroutable")
	ErrPublishNack          = errors.New("rabbitmq publish was negatively acknowledged")
	ErrPublishConfirmLost   = errors.New("rabbitmq publisher confirm channel closed")
	ErrPublishConfirmTimed  = errors.New("rabbitmq publisher confirm timed out")
	ErrPublisherUnavailable = errors.New("rabbitmq publisher is unavailable")
)

const publisherConfirmTimeout = 5 * time.Second

// clientCloseTimeout bounds shutdown when RabbitMQ cannot answer a close RPC
// while a consumer is being cancelled. The connection shutdown still closes
// every child channel and safely requeues unacknowledged deliveries.
const clientCloseTimeout = 2 * time.Second

const maxConsumeRequeueDelay = time.Second

var errDeliverySettled = errors.New("rabbitmq delivery was settled by handler")

type Client struct {
	cfg     config.RabbitMQConfig
	enabled bool
	mu      sync.RWMutex
	conn    *amqp.Connection
}

type QueueOptions struct {
	DeadLetterExchange   string
	DeadLetterRoutingKey string
	MessageTTL           time.Duration
	Expires              time.Duration
}

type ExchangeOptions struct {
	Name       string
	Type       string
	Durable    bool
	AutoDelete bool
	Arguments  map[string]any
}

type Binding struct {
	Exchange   string
	Queue      string
	RoutingKey string
}

type Topology struct {
	Exchanges []ExchangeOptions
	Queues    []QueueDeclaration
	Bindings  []Binding
}

type QueueDeclaration struct {
	Name    string
	Options QueueOptions
}

type PublishOptions struct {
	Exchange   string
	RoutingKey string
	MessageID  string
	Payload    any
	Delay      time.Duration
	Headers    map[string]any
}

// RawPublishOptions carries a pre-encoded payload for a protocol that owns
// its codec. It is transport-only: Publish and PublishJSON keep the generic
// encoding/json contract used by existing notification and file consumers.
type RawPublishOptions struct {
	Exchange   string
	RoutingKey string
	MessageID  string
	Body       []byte
	Delay      time.Duration
	Headers    map[string]any
}

type Delivery[T any] struct {
	Message     T
	MessageID   string
	Body        []byte
	Headers     map[string]any
	Redelivered bool
	RetryCount  int
	ack         func(bool) error
	nack        func(bool, bool) error
	reject      func(bool) error
}

// JSONDecoder is deliberately injectable for protocols with a stricter
// envelope contract than ordinary application messages. The callback receives
// no body so a rejection cannot accidentally turn hostile content into a log
// or metric label.
type JSONDecoder[T any] func([]byte) (T, error)

func (d Delivery[T]) Ack(multiple bool) error {
	if d.ack == nil {
		return nil
	}
	return d.ack(multiple)
}

func (d Delivery[T]) Nack(multiple bool, requeue bool) error {
	if d.nack == nil {
		return nil
	}
	return d.nack(multiple, requeue)
}

func (d Delivery[T]) Reject(requeue bool) error {
	if d.reject == nil {
		return nil
	}
	return d.reject(requeue)
}

func New(cfg config.RabbitMQConfig) (*Client, error) {
	client := &Client{cfg: cfg, enabled: cfg.Enabled}
	if !cfg.Enabled {
		return client, nil
	}
	if err := client.connect(); err != nil {
		return nil, err
	}
	return client, nil
}

func Disabled() *Client {
	return &Client{}
}

func (c *Client) Enabled() bool {
	if c == nil || !c.enabled {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && !c.conn.IsClosed()
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	conn := c.conn
	// Closing the connection closes all dedicated child channels. Clear the
	// connection snapshot before its bounded AMQP round trip.
	c.conn = nil
	c.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.CloseDeadline(time.Now().Add(clientCloseTimeout))
}

func (c *Client) Reconnect(ctx context.Context) error {
	if c == nil || !c.enabled {
		return nil
	}
	_ = c.Close()
	if ctx != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
	return c.connect()
}

func (c *Client) DeclareDirectExchange(name string) error {
	return c.DeclareExchange(ExchangeOptions{Name: name, Type: "direct", Durable: true})
}

func (c *Client) DeclareExchange(opts ExchangeOptions) error {
	if c == nil || !c.enabled {
		return nil
	}
	ch, err := c.topologyChannel()
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()
	return declareExchange(ch, opts)
}

func declareExchange(ch *amqp.Channel, opts ExchangeOptions) error {
	if opts.Type == "" {
		opts.Type = "direct"
	}
	if !opts.Durable {
		opts.Durable = true
	}
	return ch.ExchangeDeclare(opts.Name, opts.Type, opts.Durable, opts.AutoDelete, false, false, amqp.Table(opts.Arguments))
}

func (c *Client) DeclareQueue(name string, opts QueueOptions, exchange string, routingKey string) error {
	if c == nil || !c.enabled {
		return nil
	}
	ch, err := c.topologyChannel()
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()
	if err := declareQueue(ch, name, opts); err != nil {
		return err
	}
	return ch.QueueBind(name, routingKey, exchange, false, nil)
}

func (c *Client) PublishJSON(ctx context.Context, exchange, routingKey, messageID string, payload any, delay time.Duration) error {
	return c.Publish(ctx, PublishOptions{Exchange: exchange, RoutingKey: routingKey, MessageID: messageID, Payload: payload, Delay: delay})
}

func (c *Client) Publish(ctx context.Context, opts PublishOptions) error {
	if c == nil || !c.Enabled() {
		return nil
	}
	body, err := marshalGenericJSON(opts.Payload)
	if err != nil {
		return err
	}
	return c.publishRaw(ctx, RawPublishOptions{
		Exchange:   opts.Exchange,
		RoutingKey: opts.RoutingKey,
		MessageID:  opts.MessageID,
		Body:       body,
		Delay:      opts.Delay,
		Headers:    opts.Headers,
	}, false)
}

// marshalGenericJSON preserves the long-standing ordinary RabbitMQ payload
// contract. Protocol-specific codecs (such as DG5's strict Sonic envelope)
// must pre-encode through PublishRaw instead of changing unrelated consumers.
func marshalGenericJSON(payload any) ([]byte, error) {
	return stdjson.Marshal(payload)
}

// PublishRaw sends a protocol-owned already-encoded body while retaining the
// same durable publisher-confirm and mandatory-return semantics as Publish.
// Unlike the optional generic Publish path, raw protocols are required: a
// Close/Reconnect race is an error, never a successful no-op.
func (c *Client) PublishRaw(ctx context.Context, opts RawPublishOptions) error {
	return c.publishRaw(ctx, opts, true)

}

func (c *Client) publishRaw(ctx context.Context, opts RawPublishOptions, required bool) error {
	if c == nil || !c.enabled {
		if required {
			return fmt.Errorf("%w: disabled", ErrPublisherUnavailable)
		}
		return nil
	}
	if len(opts.Body) == 0 {
		return errors.New("rabbitmq raw publish body is required")
	}
	publishing := amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    opts.MessageID,
		Timestamp:    time.Now(),
		Body:         append([]byte(nil), opts.Body...),
		Headers:      amqp.Table(opts.Headers),
	}
	if opts.Delay > 0 {
		publishing.Expiration = fmt.Sprintf("%d", opts.Delay.Milliseconds())
	}

	// Publishing never shares the topology/consumer channel. A short-lived
	// dedicated channel keeps confirms and mandatory returns correlated to this
	// one message, avoiding races with concurrent consumers or publishers.
	ch, err := c.publisherChannel()
	if err != nil {
		if !required && errors.Is(err, ErrPublisherUnavailable) {
			return nil
		}
		return err
	}
	defer func() { _ = ch.Close() }()
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("enable rabbitmq publisher confirms: %w", err)
	}
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))
	confirms := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	publishCtx, cancel := publisherContext(ctx)
	defer cancel()
	if err := ch.PublishWithContext(publishCtx, opts.Exchange, opts.RoutingKey, true, false, publishing); err != nil {
		return fmt.Errorf("publish rabbitmq message: %w", err)
	}
	if err := waitForPublishOutcome(publishCtx, confirms, returns); err != nil {
		return fmt.Errorf("publish rabbitmq message %q: %w", opts.MessageID, err)
	}
	return nil
}

// QueueMessageCount is a read-only diagnostic primitive. It intentionally
// cannot mutate or remove broker topology and is used by DG5's controlled
// per-instance dead-letter diagnostics.
func (c *Client) QueueMessageCount(name string) (int, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("rabbitmq queue name is required")
	}
	ch, err := c.publisherChannel()
	if err != nil {
		return 0, err
	}
	defer func() { _ = ch.Close() }()
	queue, err := ch.QueueInspect(name)
	if err != nil {
		return 0, err
	}
	return queue.Messages, nil
}

func (c *Client) DeclareTopology(topology Topology) error {
	if c == nil || !c.Enabled() {
		return nil
	}
	ch, err := c.topologyChannel()
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()
	for _, exchange := range topology.Exchanges {
		if err := declareExchange(ch, exchange); err != nil {
			return err
		}
	}
	for _, queue := range topology.Queues {
		if err := declareQueue(ch, queue.Name, queue.Options); err != nil {
			return err
		}
	}
	for _, binding := range topology.Bindings {
		if err := ch.QueueBind(binding.Queue, binding.RoutingKey, binding.Exchange, false, nil); err != nil {
			return err
		}
	}
	return nil
}

func ConsumeJSON[T any](ctx context.Context, client *Client, queue string, consumer string, handler func(context.Context, Delivery[T]) error) error {
	return ConsumeJSONWithDecoder(ctx, client, queue, consumer, func(payload []byte) (T, error) {
		var message T
		err := stdjson.Unmarshal(payload, &message)
		return message, err
	}, nil, handler)
}

// ConsumeJSONWithDecoder retains manual ACK semantics while allowing a
// protocol to fail closed during its own decode. onDecodeRejected is
// deliberately payload-free; callers can expose a safe aggregate counter
// without retaining an attacker-controlled body.
func ConsumeJSONWithDecoder[T any](ctx context.Context, client *Client, queue string, consumer string, decoder JSONDecoder[T], onDecodeRejected func(), handler func(context.Context, Delivery[T]) error) error {
	if client == nil || !client.Enabled() {
		return nil
	}
	if decoder == nil {
		return errors.New("rabbitmq JSON decoder is required")
	}
	if handler == nil {
		return errors.New("rabbitmq JSON consumer handler is required")
	}
	ch, err := client.consumerChannel()
	if err != nil {
		return err
	}
	defer closeConsumerChannel(ch)
	deliveries, err := ch.ConsumeWithContext(ctx, queue, consumer, false, false, false, false, nil)
	if err != nil {
		return err
	}
	for {
		// amqp091-go asks the broker to cancel ConsumeWithContext, but its
		// delivery channel can remain open while that RPC is in flight. Return
		// promptly at the application boundary so shutdown cannot wait forever
		// for a Channel.Close round trip; Client.Close then closes the connection
		// and requeues anything that was not acknowledged.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case delivery, ok := <-deliveries:
			if !ok {
				return nil
			}
			message, decodeErr := decoder(delivery.Body)
			if decodeErr != nil {
				if onDecodeRejected != nil {
					onDecodeRejected()
				}
				_ = delivery.Nack(false, false)
				continue
			}
			if err := handler(ctx, Delivery[T]{
				Message:     message,
				MessageID:   delivery.MessageId,
				Body:        delivery.Body,
				Headers:     mapFromTable(delivery.Headers),
				Redelivered: delivery.Redelivered,
				RetryCount:  retryCount(delivery.Headers),
				ack:         delivery.Ack,
				nack:        delivery.Nack,
				reject:      delivery.Reject,
			}); err != nil {
				// A protocol that must publish a content-free replacement before
				// acknowledging a hostile raw body can settle this delivery itself.
				// Do not ACK/NACK it again: duplicate settlement closes an AMQP
				// channel and can hide the durable diagnostic failure.
				if errors.Is(err, errDeliverySettled) {
					continue
				}
				requeue, retryDelay := consumeRetryDisposition(err)
				if requeue && retryDelay > 0 {
					timer := time.NewTimer(retryDelay)
					select {
					case <-timer.C:
					case <-ctx.Done():
						if !timer.Stop() {
							select {
							case <-timer.C:
							default:
							}
						}
					}
				}
				_ = delivery.Nack(false, requeue)
				continue
			}
			_ = delivery.Ack(false)
		}
	}
}

// closeConsumerChannel never makes an application shutdown wait on an AMQP
// channel-close RPC. The parent connection remains the authoritative bounded
// shutdown boundary and will requeue any delivery that was not acknowledged.
func closeConsumerChannel(ch *amqp.Channel) {
	if ch == nil {
		return
	}
	go func() { _ = ch.Close() }()
}

// DeliverySettled is returned only after a handler has explicitly ACKed or
// NACKed its Delivery. It is intended for strict protocol adapters that must
// replace an untrusted raw body with a confirmed content-free diagnostic.
func DeliverySettled() error { return errDeliverySettled }

type requeueableConsumeError interface {
	Requeue() bool
}

type retryAfterConsumeError interface {
	RetryAfter() time.Duration
}

type permanentConsumeErrorMarker interface {
	Permanent() bool
}

type permanentConsumeError struct {
	err error
}

func (e permanentConsumeError) Error() string {
	if e.err == nil {
		return "permanent message consume failure"
	}
	return e.err.Error()
}

func (e permanentConsumeError) Unwrap() error { return e.err }

func (e permanentConsumeError) Permanent() bool { return true }

// PermanentConsumeError marks a malformed or otherwise permanently invalid
// broker message. Generic handler errors are retryable by default; callers
// must opt in to terminal dead-lettering through this marker.
func PermanentConsumeError(err error) error {
	if err == nil {
		return nil
	}
	return permanentConsumeError{err: err}
}

func consumeRetryDisposition(err error) (bool, time.Duration) {
	if err == nil {
		return false, 0
	}
	var permanent permanentConsumeErrorMarker
	if errors.As(err, &permanent) && permanent.Permanent() {
		return false, 0
	}
	var requeueable requeueableConsumeError
	if errors.As(err, &requeueable) && !requeueable.Requeue() {
		return false, 0
	}
	delay := maxConsumeRequeueDelay
	var retryAfter retryAfterConsumeError
	if !errors.As(err, &retryAfter) {
		return true, delay
	}
	delay = retryAfter.RetryAfter()
	if delay <= 0 {
		return true, maxConsumeRequeueDelay
	}
	if delay > maxConsumeRequeueDelay {
		delay = maxConsumeRequeueDelay
	}
	return true, delay
}

func declareQueue(ch *amqp.Channel, name string, opts QueueOptions) error {
	args := amqp.Table{}
	if opts.DeadLetterExchange != "" {
		args["x-dead-letter-exchange"] = opts.DeadLetterExchange
	}
	if opts.DeadLetterRoutingKey != "" {
		args["x-dead-letter-routing-key"] = opts.DeadLetterRoutingKey
	}
	if opts.MessageTTL > 0 {
		args["x-message-ttl"] = int64(opts.MessageTTL / time.Millisecond)
	}
	if opts.Expires > 0 {
		args["x-expires"] = int64(opts.Expires / time.Millisecond)
	}
	if len(args) == 0 {
		args = nil
	}
	_, err := ch.QueueDeclare(name, true, false, false, false, args)
	return err
}

func (c *Client) connect() error {
	if c == nil || !c.enabled {
		return nil
	}
	cfg := c.cfg
	conn, err := amqp.Dial(rabbitURL(cfg))
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
	return nil
}

func (c *Client) topologyChannel() (*amqp.Channel, error) {
	return c.openChannel("topology")
}

func (c *Client) consumerChannel() (*amqp.Channel, error) {
	ch, err := c.openChannel("consumer")
	if err != nil {
		return nil, err
	}
	prefetch := c.cfg.Prefetch
	if prefetch <= 0 {
		prefetch = 10
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		return nil, fmt.Errorf("configure dedicated rabbitmq consumer channel: %w", err)
	}
	return ch, nil
}

func (c *Client) publisherChannel() (*amqp.Channel, error) {
	return c.openChannel("publisher")
}

func (c *Client) openChannel(purpose string) (*amqp.Channel, error) {
	if c == nil || !c.enabled {
		return nil, fmt.Errorf("%w: disabled", ErrPublisherUnavailable)
	}
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()
	if conn == nil || conn.IsClosed() {
		return nil, fmt.Errorf("%w: connection is unavailable", ErrPublisherUnavailable)
	}
	ch, err := conn.Channel()
	if err != nil {
		if conn.IsClosed() {
			return nil, fmt.Errorf("%w: connection closed while opening publisher channel", ErrPublisherUnavailable)
		}
		return nil, fmt.Errorf("open dedicated rabbitmq %s channel: %w", purpose, err)
	}
	return ch, nil
}

func publisherContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, publisherConfirmTimeout)
}

func waitForPublishOutcome(ctx context.Context, confirms <-chan amqp.Confirmation, returns <-chan amqp.Return) error {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		select {
		case returned, ok := <-returns:
			if !ok {
				returns = nil
				continue
			}
			return fmt.Errorf("replyCode=%d replyText=%s exchange=%s routingKey=%s: %w", returned.ReplyCode, returned.ReplyText, returned.Exchange, returned.RoutingKey, ErrPublishUnroutable)
		case confirmation, ok := <-confirms:
			if !ok {
				return ErrPublishConfirmLost
			}
			if !confirmation.Ack {
				return ErrPublishNack
			}
			// RabbitMQ emits basic.return before the corresponding confirm. Check a
			// queued return once more before accepting the positive confirmation.
			select {
			case returned, ok := <-returns:
				if ok {
					return fmt.Errorf("replyCode=%d replyText=%s exchange=%s routingKey=%s: %w", returned.ReplyCode, returned.ReplyText, returned.Exchange, returned.RoutingKey, ErrPublishUnroutable)
				}
			default:
			}
			return nil
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return ErrPublishConfirmTimed
			}
			return ctx.Err()
		}
	}
}

func mapFromTable(table amqp.Table) map[string]any {
	if len(table) == 0 {
		return nil
	}
	result := make(map[string]any, len(table))
	for key, value := range table {
		result[key] = value
	}
	return result
}

func retryCount(headers amqp.Table) int {
	value, ok := headers["x-retry-count"]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint8:
		return int(typed)
	case string:
		var parsed int
		_, _ = fmt.Sscanf(typed, "%d", &parsed)
		return parsed
	default:
		return 0
	}
}

func rabbitURL(cfg config.RabbitMQConfig) string {
	if strings.TrimSpace(cfg.URL) != "" {
		return cfg.URL
	}
	host := strings.TrimSpace(cfg.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	user := strings.TrimSpace(cfg.Username)
	if user == "" {
		user = "guest"
	}
	pass := cfg.Password
	vhost := strings.TrimSpace(cfg.VHost)
	if vhost == "" {
		vhost = "/"
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%d/%s", url.QueryEscape(user), url.QueryEscape(pass), host, cfg.Port, strings.TrimPrefix(url.PathEscape(vhost), "/"))
}
