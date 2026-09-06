package infrastructure

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-framework/seven-framework-server/internal/app/notification/domain"
)

func TestResolveSMTPServerNameSupportsSecureLocalTunnel(t *testing.T) {
	if got := resolveSMTPServerName(smtpConfig{TLSServerName: " mail.example.com "}, "127.0.0.1"); got != "mail.example.com" {
		t.Fatalf("resolveSMTPServerName()=%q, want mail.example.com", got)
	}
	if got := resolveSMTPServerName(smtpConfig{}, " smtp.example.com "); got != "smtp.example.com" {
		t.Fatalf("resolveSMTPServerName() fallback=%q, want smtp.example.com", got)
	}
}

func TestSMTPDriverRejectsMissingRequiredStartTLS(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp fixture: %v", err)
	}
	defer listener.Close()

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_, _ = connection.Write([]byte("220 local fixture\r\n"))
		reader := bufio.NewReader(connection)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), "EHLO ") {
				_, _ = connection.Write([]byte("250-local fixture\r\n250 AUTH PLAIN\r\n"))
				continue
			}
			return
		}
	}()

	address := listener.Addr().(*net.TCPAddr)
	configJSON, err := json.Marshal(smtpConfig{
		Host:          "127.0.0.1",
		Port:          address.Port,
		StartTLS:      true,
		TLSServerName: "mail.example.com",
	})
	if err != nil {
		t.Fatalf("marshal smtp config: %v", err)
	}
	err = (SMTPDriver{}).Send(context.Background(), domain.DriverMessage{
		Channel: domain.Channel{ChannelName: "sender@example.com", ConfigJSON: string(configJSON)},
		Target:  "recipient@example.com",
		Subject: "test",
		Text:    "test",
	})
	if err == nil || !strings.Contains(err.Error(), "does not support required STARTTLS") {
		t.Fatalf("Send() error=%v, want required STARTTLS rejection", err)
	}
	<-serverDone
}

func TestSMTPDriverRejectsConflictingTLSModes(t *testing.T) {
	configJSON, err := json.Marshal(smtpConfig{Host: "127.0.0.1", Port: 25, UseTLS: true, StartTLS: true})
	if err != nil {
		t.Fatalf("marshal smtp config: %v", err)
	}
	err = (SMTPDriver{}).Send(context.Background(), domain.DriverMessage{
		Channel: domain.Channel{ConfigJSON: string(configJSON)},
	})
	if err == nil || !strings.Contains(err.Error(), "must not both be enabled") {
		t.Fatalf("Send() error=%v, want conflicting TLS mode rejection", err)
	}
}

func TestSMTPDriverUsesTLSNameForSNIAndAuthenticatedSession(t *testing.T) {
	certificate := newSMTPTestCertificate(t, "mail.example.com")
	observedServerName := make(chan string, 1)
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{certificate},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			observedServerName <- hello.ServerName
			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("listen direct TLS smtp fixture: %v", err)
	}
	defer listener.Close()

	serverResult := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			serverResult <- acceptErr
			return
		}
		defer connection.Close()
		serverResult <- serveAuthenticatedSMTPFixture(connection)
	}()

	address := listener.Addr().(*net.TCPAddr)
	configJSON, err := json.Marshal(smtpConfig{
		Host:          "127.0.0.1",
		Port:          address.Port,
		Username:      "sender@example.com",
		From:          "sender@example.com",
		UseTLS:        true,
		SkipVerify:    true,
		TLSServerName: "mail.example.com",
	})
	if err != nil {
		t.Fatalf("marshal direct TLS smtp config: %v", err)
	}
	if err := (SMTPDriver{}).Send(context.Background(), domain.DriverMessage{
		Channel:     domain.Channel{ConfigJSON: string(configJSON)},
		Target:      "recipient@example.com",
		Subject:     "test",
		Text:        "test",
		SecretPlain: "fixture-password",
	}); err != nil {
		t.Fatalf("Send() over direct TLS: %v", err)
	}
	if got := <-observedServerName; got != "mail.example.com" {
		t.Fatalf("TLS SNI=%q, want mail.example.com", got)
	}
	if err := <-serverResult; err != nil {
		t.Fatalf("smtp fixture: %v", err)
	}
}

func newSMTPTestCertificate(t *testing.T, serverName string) tls.Certificate {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate fixture key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: serverName},
		DNSNames:     []string{serverName},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create fixture certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: privateKey}
}

func serveAuthenticatedSMTPFixture(connection net.Conn) error {
	if _, err := connection.Write([]byte("220 direct TLS fixture\r\n")); err != nil {
		return err
	}
	reader := bufio.NewReader(connection)
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		command := strings.ToUpper(strings.TrimSpace(line))
		if inData {
			if command == "." {
				inData = false
				if _, err := connection.Write([]byte("250 queued\r\n")); err != nil {
					return err
				}
			}
			continue
		}
		switch {
		case strings.HasPrefix(command, "EHLO "):
			_, err = connection.Write([]byte("250-direct TLS fixture\r\n250-AUTH PLAIN\r\n250 OK\r\n"))
		case strings.HasPrefix(command, "AUTH PLAIN "):
			_, err = connection.Write([]byte("235 authenticated\r\n"))
		case strings.HasPrefix(command, "MAIL FROM:"):
			_, err = connection.Write([]byte("250 sender accepted\r\n"))
		case strings.HasPrefix(command, "RCPT TO:"):
			_, err = connection.Write([]byte("250 recipient accepted\r\n"))
		case command == "DATA":
			inData = true
			_, err = connection.Write([]byte("354 continue\r\n"))
		case command == "QUIT":
			_, err = connection.Write([]byte("221 bye\r\n"))
			return err
		default:
			return fmt.Errorf("unexpected smtp command %q", command)
		}
		if err != nil {
			return err
		}
	}
}
