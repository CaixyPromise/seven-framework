package infrastructure

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/app/notification/domain"
	cacheinfra "github.com/CaixyPromise/seven-framework/seven-framework-server/internal/infrastructure/cache"
	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/infrastructure/security/outboundurl"
)

type DriverRegistry struct {
	drivers map[string]domain.ChannelDriver
}

func NewDriverRegistry(cache cacheinfra.Manager) *DriverRegistry {
	return NewDriverRegistryWithHTTPClient(cache, nil)
}

// NewDriverRegistryWithOutboundGuard wires fixed-provider application traffic
// through the same SSRF guard used by URL-channel validation. A nil guard
// intentionally leaves G4 application adapters unavailable rather than
// creating an unguarded fallback HTTP client.
func NewDriverRegistryWithOutboundGuard(cache cacheinfra.Manager, guard *outboundurl.OutboundURLGuard, policies ...outboundurl.PolicyResolver) *DriverRegistry {
	if guard == nil {
		return NewDriverRegistryWithHTTPClient(cache, nil)
	}
	var resolver outboundurl.PolicyResolver
	if len(policies) > 0 {
		resolver = policies[0]
	}
	clientFor := func(policy outboundurl.Policy) enterpriseHTTPDoer {
		return guard.HTTPClient(policy)
	}
	return newDriverRegistry(cache, clientFor(outboundurl.Policy{Mode: outboundurl.ModePublic}), clientFor, resolver)
}

// NewDriverRegistryWithHTTPClient exists for isolated adapter tests. Runtime
// wiring must call NewDriverRegistryWithOutboundGuard instead.
func NewDriverRegistryWithHTTPClient(cache cacheinfra.Manager, client enterpriseHTTPDoer) *DriverRegistry {
	clientFor := func(outboundurl.Policy) enterpriseHTTPDoer { return client }
	return newDriverRegistry(cache, client, clientFor, nil)
}

func newDriverRegistry(cache cacheinfra.Manager, client enterpriseHTTPDoer, staticClients staticHTTPClientFactory, policies outboundurl.PolicyResolver) *DriverRegistry {
	drivers := map[string]domain.ChannelDriver{
		domain.ChannelTypeMock:  &MockDriver{cache: cache},
		domain.ChannelTypeEmail: SMTPDriver{},
	}
	if client != nil {
		tokens := newEnterpriseTokenCache()
		drivers[domain.ChannelTypeFeishuApp] = &enterpriseApplicationDriver{provider: domain.ChannelTypeFeishuApp, client: client, tokens: tokens}
		drivers[domain.ChannelTypeWeComApp] = &enterpriseApplicationDriver{provider: domain.ChannelTypeWeComApp, client: client, tokens: tokens}
		drivers[domain.ChannelTypeHTTPConnector] = &staticHTTPDriver{channelType: domain.ChannelTypeHTTPConnector, clients: staticClients, policies: policies}
		drivers[domain.ChannelTypeFeishuWebhook] = &staticHTTPDriver{channelType: domain.ChannelTypeFeishuWebhook, clients: staticClients, policies: policies}
		drivers[domain.ChannelTypeWeComWebhook] = &staticHTTPDriver{channelType: domain.ChannelTypeWeComWebhook, clients: staticClients, policies: policies}
	}
	return &DriverRegistry{drivers: drivers}
}

func (r *DriverRegistry) Driver(channelType string) domain.ChannelDriver {
	if r == nil {
		return nil
	}
	return r.drivers[strings.ToUpper(strings.TrimSpace(channelType))]
}

type MockDriver struct {
	cache cacheinfra.Manager
}

func (d *MockDriver) Send(ctx context.Context, message domain.DriverMessage) error {
	if strings.TrimSpace(message.Target) == "" {
		return fmt.Errorf("notification target must not be empty")
	}
	if d == nil || d.cache == nil {
		return nil
	}
	prefix := "notification:mock:capture"
	var cfg map[string]any
	if err := json.Unmarshal([]byte(message.Channel.ConfigJSON), &cfg); err == nil {
		if value, ok := cfg["capturePrefix"].(string); ok && strings.TrimSpace(value) != "" {
			prefix = strings.TrimSpace(value)
		}
	}
	payload, _ := json.Marshal(map[string]any{
		"target":   message.Target,
		"subject":  message.Subject,
		"text":     message.Text,
		"html":     message.HTML,
		"markdown": message.Markdown,
		"sentAt":   time.Now().UTC().Format(time.RFC3339Nano),
	})
	return d.cache.SetBytes(ctx, prefix+":"+strings.ToLower(strings.TrimSpace(message.Target)), payload, 10*time.Minute)
}

type SMTPDriver struct{}

type smtpConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	From       string `json:"from"`
	UseTLS     bool   `json:"useTls"`
	StartTLS   bool   `json:"startTls"`
	Timeout    string `json:"timeout"`
	SkipVerify bool   `json:"skipVerify"`
}

func (SMTPDriver) Send(ctx context.Context, message domain.DriverMessage) error {
	var cfg smtpConfig
	if err := json.Unmarshal([]byte(message.Channel.ConfigJSON), &cfg); err != nil {
		return fmt.Errorf("parse smtp channel config: %w", err)
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("smtp host must not be empty")
	}
	if cfg.Port <= 0 {
		cfg.Port = 25
	}
	timeout := 10 * time.Second
	if strings.TrimSpace(cfg.Timeout) != "" {
		if parsed, err := time.ParseDuration(cfg.Timeout); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	host := strings.TrimSpace(cfg.Host)
	addr := fmt.Sprintf("%s:%d", host, cfg.Port)
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	var client *smtp.Client
	if cfg.UseTLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, InsecureSkipVerify: cfg.SkipVerify}) //nolint:gosec
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return err
		}
		client, err = smtp.NewClient(tlsConn, host)
	} else {
		client, err = smtp.NewClient(conn, host)
	}
	if err != nil {
		return err
	}
	defer client.Quit()
	if cfg.StartTLS {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(&tls.Config{ServerName: host, InsecureSkipVerify: cfg.SkipVerify}); err != nil { //nolint:gosec
				return err
			}
		}
	}
	if strings.TrimSpace(cfg.Username) != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.Username, message.SecretPlain, host)); err != nil {
			return err
		}
	}
	from := choose(cfg.From, message.Channel.ChannelName)
	if !strings.Contains(from, "@") && strings.Contains(cfg.Username, "@") {
		from = cfg.Username
	}
	fromAddress := extractAddress(from)
	if err := client.Mail(fromAddress); err != nil {
		return err
	}
	if err := client.Rcpt(message.Target); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte(buildMIMEMessage(from, message.Target, message.Subject, message.Text, message.HTML)))
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	return err
}

func buildMIMEMessage(from, to, subject, textBody, htmlBody string) string {
	boundary := "seven-notification-boundary"
	var builder strings.Builder
	builder.WriteString("From: " + from + "\r\n")
	builder.WriteString("To: " + to + "\r\n")
	builder.WriteString("Subject: " + subject + "\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: multipart/alternative; boundary=\"" + boundary + "\"\r\n\r\n")
	builder.WriteString("--" + boundary + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + textBody + "\r\n")
	builder.WriteString("--" + boundary + "\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n" + htmlBody + "\r\n")
	builder.WriteString("--" + boundary + "--\r\n")
	return builder.String()
}

func choose(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func extractAddress(value string) string {
	if parsed, err := mail.ParseAddress(value); err == nil {
		return parsed.Address
	}
	return strings.TrimSpace(value)
}
