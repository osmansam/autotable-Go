package configs

import "testing"

func TestCollaborationInvitationMailConfigRequiresSecureTransport(t *testing.T) {
	values := map[string]string{"INVITATION_SMTP_HOST": "smtp.example.test", "INVITATION_SMTP_FROM": "noreply@example.test", "INVITATION_SMTP_TLS_MODE": "starttls", "INVITATION_SMTP_PORT": "587", "INVITATION_PANEL_URL": "https://tenant.example.test", "INVITATION_DELIVERY_KEYS_JSON": "configured-by-deployment", "INVITATION_DELIVERY_ACTIVE_KEY_ID": "test", "INVITATION_SMTP_USERNAME": "", "INVITATION_SMTP_PASSWORD": ""}
	for key, value := range values {
		t.Setenv(key, value)
	}
	if _, err := LoadInvitationMailConfig(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ key, value string }{{"INVITATION_SMTP_TLS_MODE", "none"}, {"INVITATION_SMTP_PORT", "bad"}, {"INVITATION_PANEL_URL", "http://tenant.example.test"}, {"INVITATION_PANEL_URL", "https://user:password@tenant.example.test"}, {"INVITATION_SMTP_FROM", "sender@example.test\r\nBcc:other@example.test"}} {
		t.Run(tc.key+tc.value, func(t *testing.T) {
			t.Setenv(tc.key, tc.value)
			if _, err := LoadInvitationMailConfig(); err == nil {
				t.Fatal("insecure mail configuration accepted")
			}
		})
	}
}
