package configs

import (
	"os"
	"strings"
)

type CollaborationConfig struct{ MembersEnabled, InvitationsEnabled, RecoveryEnabled bool }

func GetCollaborationConfig() CollaborationConfig {
	enabled := func(key string) bool { return strings.EqualFold(strings.TrimSpace(os.Getenv(key)), "true") }
	return CollaborationConfig{MembersEnabled: enabled("COLLABORATION_MEMBERS_ENABLED"), InvitationsEnabled: enabled("COLLABORATION_INVITATIONS_ENABLED"), RecoveryEnabled: enabled("COLLABORATION_RECOVERY_ENABLED")}
}
