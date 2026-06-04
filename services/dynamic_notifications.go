package services

import (
	"context"
	"log"
	"time"

	"github.com/osmansam/autotableGo/repositories"
)

func EnsureDynamicNotificationIndexes(ctx context.Context) {
	indexCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := repositories.NewDynamicRepository().EnsureNotificationIndexesForAllProjects(indexCtx); err != nil {
		log.Printf("notifications: failed to ensure indexes: %v", err)
	}
}
