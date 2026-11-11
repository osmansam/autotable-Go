package models

import (
	"encoding/json"
	"fmt"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"strings"
	"time"
)

// Value is opaque authored JSON. BSON stores its bytes; JSON responses embed the
// original object rather than transforming case-sensitive theme/effect keys.
type VisualComponentDesign struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID  string             `bson:"tenantId" json:"-"`
	ProjectID string             `bson:"projectId" json:"-"`
	Name      string             `bson:"name" json:"name"`
	Value     json.RawMessage    `bson:"value" json:"value"`
	Revision  int64              `bson:"revision" json:"revision"`
	CreatedAt time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time          `bson:"updatedAt" json:"updatedAt"`
}

func ValidateVisualComponentDesign(value *VisualComponentDesign) error {
	value.Name = strings.TrimSpace(value.Name)
	if value.Name == "" || len(value.Name) > 160 {
		return fmt.Errorf("design name must contain 1 to 160 characters")
	}
	if len(value.Value) > 2*1024*1024 {
		return fmt.Errorf("visual design exceeds 2 MiB")
	}
	var envelope struct {
		Document struct {
			Version int    `json:"version"`
			ID      string `json:"id"`
			Root    struct {
				ID       string             `json:"id"`
				Type     string             `json:"type"`
				Children *[]json.RawMessage `json:"children"`
			} `json:"root"`
		} `json:"document"`
	}
	if err := json.Unmarshal(value.Value, &envelope); err != nil {
		return fmt.Errorf("invalid visual design JSON")
	}
	doc := envelope.Document
	if (doc.Version != 1 && doc.Version != 2) || doc.ID == "" || doc.Root.ID == "" || doc.Root.Type != "section" || doc.Root.Children == nil {
		return fmt.Errorf("invalid visual document envelope")
	}
	return nil
}
