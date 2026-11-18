package configs

import "testing"

func TestCollaborationFlagsDefaultDisabled(t *testing.T) {
	for _, key := range []string{"COLLABORATION_MEMBERS_ENABLED", "COLLABORATION_INVITATIONS_ENABLED", "COLLABORATION_RECOVERY_ENABLED"} {
		t.Setenv(key, "")
	}
	if got := GetCollaborationConfig(); got.MembersEnabled || got.InvitationsEnabled || got.RecoveryEnabled {
		t.Fatalf("unsafe default: %#v", got)
	}
	t.Setenv("COLLABORATION_MEMBERS_ENABLED", "garbage")
	if GetCollaborationConfig().MembersEnabled {
		t.Fatal("malformed flag enabled feature")
	}
}
