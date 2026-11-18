package configs

import (
	"errors"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type InvitationMailConfig struct {
	Host                                                                  string
	Port                                                                  int
	Username, Password, From, TLSMode, PanelURL, KeyringJSON, ActiveKeyID string
}

func LoadInvitationMailConfig() (InvitationMailConfig, error) {
	cfg := InvitationMailConfig{Host: strings.TrimSpace(os.Getenv("INVITATION_SMTP_HOST")), Username: os.Getenv("INVITATION_SMTP_USERNAME"), Password: os.Getenv("INVITATION_SMTP_PASSWORD"), From: strings.TrimSpace(os.Getenv("INVITATION_SMTP_FROM")), TLSMode: strings.TrimSpace(os.Getenv("INVITATION_SMTP_TLS_MODE")), PanelURL: strings.TrimSpace(os.Getenv("INVITATION_PANEL_URL")), KeyringJSON: os.Getenv("INVITATION_DELIVERY_KEYS_JSON"), ActiveKeyID: os.Getenv("INVITATION_DELIVERY_ACTIVE_KEY_ID")}
	if cfg.TLSMode == "" {
		cfg.TLSMode = "starttls"
	}
	cfg.Port = 587
	if cfg.TLSMode == "implicit" {
		cfg.Port = 465
	}
	if raw := os.Getenv("INVITATION_SMTP_PORT"); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil {
			return cfg, errors.New("invalid INVITATION_SMTP_PORT")
		}
		cfg.Port = port
	}
	if cfg.Host == "" || strings.ContainsAny(cfg.Host, "/\r\n") || cfg.Port < 1 || cfg.Port > 65535 || (cfg.TLSMode != "starttls" && cfg.TLSMode != "implicit") {
		return cfg, errors.New("SMTP host, valid port and TLS mode are required")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil || strings.ContainsAny(cfg.From, "\r\n") {
		return cfg, errors.New("valid INVITATION_SMTP_FROM is required")
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return cfg, errors.New("both SMTP authentication fields are required")
	}
	panel, err := url.Parse(cfg.PanelURL)
	if err != nil || panel.Scheme != "https" || panel.Hostname() == "" || panel.User != nil || panel.RawQuery != "" || panel.Fragment != "" || (panel.Path != "" && panel.Path != "/") {
		return cfg, errors.New("INVITATION_PANEL_URL must be an HTTPS origin")
	}
	if cfg.KeyringJSON == "" || cfg.ActiveKeyID == "" {
		return cfg, errors.New("invitation delivery keyring and active key ID are required")
	}
	return cfg, nil
}
