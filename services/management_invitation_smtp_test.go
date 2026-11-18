package services

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
)

func smtpInvitationFixture(t *testing.T, secure, rejectRecipient, dropAfterData bool) (configs.InvitationMailConfig, *tls.Config, <-chan []string) {
	t.Helper()
	var listener net.Listener
	var clientTLS *tls.Config
	var err error
	if secure {
		certificateServer := httptest.NewTLSServer(nil)
		serverTLS := certificateServer.TLS.Clone()
		roots := x509.NewCertPool()
		roots.AddCert(certificateServer.Certificate())
		certificateServer.Close()
		listener, err = tls.Listen("tcp", "127.0.0.1:0", serverTLS)
		clientTLS = &tls.Config{ServerName: "127.0.0.1", RootCAs: roots, MinVersion: tls.VersionTLS12}
	} else {
		listener, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	host, portString, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portString)
	mode := "starttls"
	if secure {
		mode = "implicit"
	}
	completed := make(chan []string, 1)
	go func() {
		commands := []string{}
		defer func() { completed <- commands }()
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_ = connection.SetDeadline(time.Now().Add(5 * time.Second))
		protocol := textproto.NewConn(connection)
		if err := protocol.PrintfLine("220 localhost SMTP"); err != nil {
			return
		}
		for {
			line, err := protocol.ReadLine()
			if err != nil {
				return
			}
			commands = append(commands, line)
			switch {
			case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
				_ = protocol.PrintfLine("250 localhost")
			case strings.HasPrefix(line, "MAIL FROM"):
				_ = protocol.PrintfLine("250 sender accepted")
			case strings.HasPrefix(line, "RCPT TO"):
				if rejectRecipient {
					_ = protocol.PrintfLine("550 rejected")
				} else {
					_ = protocol.PrintfLine("250 recipient accepted")
				}
			case line == "DATA":
				_ = protocol.PrintfLine("354 data")
				_, _ = io.Copy(io.Discard, protocol.DotReader())
				_ = protocol.PrintfLine("250 queued")
				if dropAfterData {
					return
				}
			case line == "QUIT":
				_ = protocol.PrintfLine("221 bye")
				return
			default:
				_ = protocol.PrintfLine("500 unsupported")
			}
		}
	}()
	return configs.InvitationMailConfig{Host: host, Port: port, TLSMode: mode, From: "noreply@example.test"}, clientTLS, completed
}
func invitationSMTPMessage() models.InvitationMessage {
	return models.InvitationMessage{To: "collaborator@example.test", MessageID: "test-id@example.test", Subject: "Project invitation", TextBody: "Task-owned SMTP fixture message."}
}
func TestCollaborationInvitationSMTPRequiresTLS(t *testing.T) {
	cfg, _, completed := smtpInvitationFixture(t, false, false, false)
	cfg.Username = "test-user"
	cfg.Password = "test-password"
	err := NewSMTPInvitationTransport(cfg).Send(context.Background(), invitationSMTPMessage())
	if err == nil {
		t.Fatal("SMTP downgrade allowed")
	}
	for _, command := range <-completed {
		if strings.HasPrefix(command, "AUTH") || strings.HasPrefix(command, "MAIL") {
			t.Fatal("credentials or mail sent before TLS")
		}
	}
}
func TestCollaborationInvitationSMTPRejectsUntrustedCertificate(t *testing.T) {
	cfg, _, completed := smtpInvitationFixture(t, true, false, false)
	if err := NewSMTPInvitationTransport(cfg).Send(context.Background(), invitationSMTPMessage()); err == nil {
		t.Fatal("untrusted TLS certificate accepted")
	}
	<-completed
}
func TestCollaborationInvitationSMTPRecipientRejection(t *testing.T) {
	cfg, trusted, completed := smtpInvitationFixture(t, true, true, false)
	transport := NewSMTPInvitationTransport(cfg)
	transport.tlsConfig = trusted
	if err := transport.Send(context.Background(), invitationSMTPMessage()); err == nil {
		t.Fatal("recipient rejection reported as sent")
	}
	<-completed
}
func TestCollaborationInvitationSMTPAcceptedDataSurvivesQuitLoss(t *testing.T) {
	cfg, trusted, completed := smtpInvitationFixture(t, true, false, true)
	transport := NewSMTPInvitationTransport(cfg)
	transport.tlsConfig = trusted
	if err := transport.Send(context.Background(), invitationSMTPMessage()); err != nil {
		t.Fatalf("accepted SMTP message reported failed after QUIT loss: %v", err)
	}
	<-completed
}
func TestCollaborationInvitationSMTPContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	connectionDone := make(chan struct{})
	go func() {
		defer close(connectionDone)
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		defer connection.Close()
		_, _ = io.Copy(io.Discard, connection)
	}()
	host, rawPort, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = NewSMTPInvitationTransport(configs.InvitationMailConfig{Host: host, Port: port, TLSMode: "starttls", From: "noreply@example.test"}).Send(ctx, invitationSMTPMessage())
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatal("SMTP ignored context deadline")
	}
	<-connectionDone
}
