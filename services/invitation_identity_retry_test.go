package services

import (
	"errors"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/mongo"
	"testing"
)

func TestCollaborationIdentityGuardPreservesTransactionRetry(t *testing.T) {
	original := mongo.CommandError{Code: 112, Message: "write conflict", Labels: []string{"TransientTransactionError"}}
	var labelled interface{ HasErrorLabel(string) bool }
	if err := invitationOwnershipGuardError(original); !errors.As(err, &labelled) || !labelled.HasErrorLabel("TransientTransactionError") {
		t.Fatal("account proof lost transaction retry label")
	}
	if err := invitationOwnershipGuardError(repositories.ErrCollaborationInactive); !errors.Is(err, ErrInvitationOwnership) {
		t.Fatal("inactive account guard did not deny ownership proof")
	}
}
