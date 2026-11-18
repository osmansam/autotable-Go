package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type SMTPInvitationTransport struct {
	config    configs.InvitationMailConfig
	tlsConfig *tls.Config
}

func NewSMTPInvitationTransport(cfg configs.InvitationMailConfig) *SMTPInvitationTransport {
	return &SMTPInvitationTransport{config: cfg, tlsConfig: &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}}
}
func (s *SMTPInvitationTransport) Send(ctx context.Context, message models.InvitationMessage) error {
	cfg := s.config
	if cfg.TLSMode != "starttls" && cfg.TLSMode != "implicit" {
		return errors.New("TLS is required for invitation mail")
	}
	from, err := mail.ParseAddress(cfg.From)
	if err != nil {
		return errors.New("invalid sender")
	}
	to, err := mail.ParseAddress(message.To)
	if err != nil || to.Address != message.To || strings.ContainsAny(message.To+message.MessageID+message.Subject+cfg.From, "\r\n") {
		return errors.New("invalid email headers")
	}
	address := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var connection net.Conn
	if cfg.TLSMode == "implicit" {
		connection, err = (&tls.Dialer{NetDialer: dialer, Config: s.tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		connection, err = dialer.DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return errors.New("SMTP connection failed")
	}
	defer connection.Close()
	stop := context.AfterFunc(ctx, func() { _ = connection.Close() })
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	if existing, ok := ctx.Deadline(); ok && existing.Before(deadline) {
		deadline = existing
	}
	_ = connection.SetDeadline(deadline)
	client, err := smtp.NewClient(connection, cfg.Host)
	if err != nil {
		return errors.New("SMTP greeting failed")
	}
	defer client.Close()
	if cfg.TLSMode == "starttls" {
		if supported, _ := client.Extension("STARTTLS"); !supported {
			return errors.New("SMTP server does not offer required TLS")
		}
		if err := client.StartTLS(s.tlsConfig); err != nil {
			return errors.New("SMTP TLS validation failed")
		}
	}
	if cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return errors.New("SMTP authentication failed")
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return errors.New("SMTP sender rejected")
	}
	if err := client.Rcpt(to.Address); err != nil {
		return errors.New("SMTP recipient rejected")
	}
	writer, err := client.Data()
	if err != nil {
		return errors.New("SMTP data rejected")
	}
	var body bytes.Buffer
	encoded := quotedprintable.NewWriter(&body)
	if _, err := encoded.Write([]byte(message.TextBody)); err != nil {
		return err
	}
	if err := encoded.Close(); err != nil {
		return err
	}
	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nMessage-ID: <%s>\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", from.String(), to.String(), message.MessageID, mime.QEncoding.Encode("UTF-8", message.Subject))
	if _, err := writer.Write(append([]byte(headers), body.Bytes()...)); err != nil {
		_ = writer.Close()
		return errors.New("SMTP send failed")
	}
	if err := writer.Close(); err != nil {
		return errors.New("SMTP send rejected")
	}
	// DATA's final 250 reply confirms SMTP acceptance. A lost QUIT reply must
	// not turn an already accepted message into a reported delivery failure.
	_ = client.Quit()
	return nil
}
