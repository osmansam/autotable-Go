package repositories

import (
	"context"
	"fmt"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"time"
)

func (r *DynamicRepository) EnsureExternalAPICredentialGrantIndexes(ctx context.Context) error {
	_, err := r.globalCollection("external_api_credential_grants").Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "credentialId", Value: 1}, {Key: "projectId", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "tenantId", Value: 1}, {Key: "projectId", Value: 1}}},
	})
	return err
}
func credentialIDs(tenantID, credentialID string) (primitive.ObjectID, primitive.ObjectID, error) {
	t, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return t, primitive.NilObjectID, err
	}
	id, err := primitive.ObjectIDFromHex(credentialID)
	return t, id, err
}
func projectCredentialScopeFilter() bson.M {
	return bson.M{"$in": bson.A{"", models.ExternalAPICredentialScopeProject, nil}}
}
func (r *DynamicRepository) GetTenantExternalAPICredential(ctx context.Context, tenantID, credentialID string) (*models.ExternalAPICredential, error) {
	t, id, err := credentialIDs(tenantID, credentialID)
	if err != nil {
		return nil, err
	}
	var c models.ExternalAPICredential
	if err = r.externalAPICredentialsCollection().FindOne(ctx, bson.M{"_id": id, "tenantId": t}).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}
func (r *DynamicRepository) GetExternalAPICredential(ctx context.Context, tenantID, projectID, credentialID string) (*models.ExternalAPICredential, error) {
	p, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return nil, err
	}
	t, id, err := credentialIDs(tenantID, credentialID)
	if err != nil {
		return nil, err
	}
	var credential models.ExternalAPICredential
	err = r.externalAPICredentialsCollection().FindOne(ctx, bson.M{"_id": id, "tenantId": t, "$or": bson.A{
		bson.M{"scope": models.ExternalAPICredentialScopeTenant},
		bson.M{"scope": projectCredentialScopeFilter(), "projectId": p},
	}}).Decode(&credential)
	if err != nil {
		return nil, err
	}
	c := &credential
	switch c.EffectiveScope() {
	case models.ExternalAPICredentialScopeProject:
		if c.ProjectID != p {
			return nil, mongo.ErrNoDocuments
		}
	case models.ExternalAPICredentialScopeTenant:
		err = r.globalCollection("external_api_credential_grants").FindOne(ctx, bson.M{"tenantId": c.TenantID, "credentialId": c.ID, "projectId": p}).Err()
		if err != nil {
			return nil, err
		}
	default:
		return nil, mongo.ErrNoDocuments
	}
	return c, nil
}
func (r *DynamicRepository) ListTenantExternalAPICredentials(ctx context.Context, tenantID string) ([]models.ExternalAPICredential, error) {
	t, err := primitive.ObjectIDFromHex(tenantID)
	if err != nil {
		return nil, err
	}
	cursor, err := r.externalAPICredentialsCollection().Find(ctx, bson.M{"tenantId": t})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	result := []models.ExternalAPICredential{}
	if err = cursor.All(ctx, &result); err != nil {
		return nil, err
	}
	for i := range result {
		result[i].Scope = result[i].EffectiveScope()
		result[i].GrantedProjectIDs = []primitive.ObjectID{}
		if result[i].Scope != models.ExternalAPICredentialScopeTenant {
			continue
		}
		cur, e := r.globalCollection("external_api_credential_grants").Find(ctx, bson.M{"tenantId": t, "credentialId": result[i].ID})
		if e != nil {
			return nil, e
		}
		var grants []models.ExternalAPICredentialGrant
		e = cur.All(ctx, &grants)
		cur.Close(ctx)
		if e != nil {
			return nil, e
		}
		for _, g := range grants {
			result[i].GrantedProjectIDs = append(result[i].GrantedProjectIDs, g.ProjectID)
		}
	}
	return result, nil
}
func (r *DynamicRepository) ListExternalAPICredentials(ctx context.Context, tenantID, projectID string) ([]models.ExternalAPICredential, error) {
	p, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return nil, err
	}
	all, err := r.ListTenantExternalAPICredentials(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := []models.ExternalAPICredential{}
	for _, c := range all {
		if c.Scope == models.ExternalAPICredentialScopeProject && c.ProjectID == p {
			out = append(out, c)
		}
		if c.Scope == models.ExternalAPICredentialScopeTenant {
			for _, id := range c.GrantedProjectIDs {
				if id == p {
					c.GrantedProjectIDs = []primitive.ObjectID{p}
					out = append(out, c)
					break
				}
			}
		}
	}
	return out, nil
}
func (r *DynamicRepository) RevokeExternalAPICredential(ctx context.Context, tenantID, projectID, credentialID string, now time.Time) (*mongo.UpdateResult, error) {
	t, id, err := credentialIDs(tenantID, credentialID)
	if err != nil {
		return nil, err
	}
	p, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return nil, err
	}
	return r.externalAPICredentialsCollection().UpdateOne(ctx, bson.M{"_id": id, "tenantId": t, "projectId": p, "scope": projectCredentialScopeFilter()}, bson.M{"$set": bson.M{"revokedAt": now, "updatedAt": now}})
}
func (r *DynamicRepository) RevokeTenantExternalAPICredential(ctx context.Context, tenantID, credentialID string, now time.Time) (*mongo.UpdateResult, error) {
	t, id, err := credentialIDs(tenantID, credentialID)
	if err != nil {
		return nil, err
	}
	return r.externalAPICredentialsCollection().UpdateOne(ctx, bson.M{"_id": id, "tenantId": t, "scope": models.ExternalAPICredentialScopeTenant}, bson.M{"$set": bson.M{"revokedAt": now, "updatedAt": now}})
}
func ValidateSharedExternalAPICredential(c *models.ExternalAPICredential, now time.Time) error {
	if c.EffectiveScope() != models.ExternalAPICredentialScopeTenant {
		return fmt.Errorf("credential must be tenant scope; promote or replace project credential")
	}
	if c.RevokedAt != nil {
		return fmt.Errorf("credential is revoked")
	}
	if !c.ExpiresAt.IsZero() && !c.ExpiresAt.After(now) {
		return fmt.Errorf("credential is expired")
	}
	return nil
}

// touchCredentialGrantProject serializes grant creation with project deactivation/deletion.
func (r *DynamicRepository) touchCredentialGrantProject(ctx context.Context, t, p primitive.ObjectID) error {
	result, err := r.globalCollection("projects").UpdateOne(ctx, bson.M{"_id": p, "tenantId": t, "isActive": true}, bson.M{"$inc": bson.M{"externalAPICredentialGrantVersion": 1}})
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("active project in this tenant required: %w", mongo.ErrNoDocuments)
	}
	return nil
}
func (r *DynamicRepository) GrantExternalAPICredential(ctx context.Context, tenantID, credentialID, projectID string, grantor primitive.ObjectID) error {
	p, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return err
	}
	if err = r.EnsureExternalAPICredentialGrantIndexes(ctx); err != nil {
		return err
	}
	return r.WithTransaction(ctx, func(tx mongo.SessionContext) error {
		c, err := r.GetTenantExternalAPICredential(tx, tenantID, credentialID)
		if err != nil {
			return err
		}
		if err = ValidateSharedExternalAPICredential(c, time.Now()); err != nil {
			return err
		}
		if err = r.touchCredentialGrantProject(tx, c.TenantID, p); err != nil {
			return err
		}
		return r.upsertExternalCredentialGrant(tx, c.TenantID, c.ID, p, grantor)
	})
}
func (r *DynamicRepository) upsertExternalCredentialGrant(ctx context.Context, t, id, p, grantor primitive.ObjectID) error {
	filter := bson.M{"tenantId": t, "credentialId": id, "projectId": p}
	_, err := r.globalCollection("external_api_credential_grants").UpdateOne(ctx, filter, bson.M{"$setOnInsert": models.ExternalAPICredentialGrant{TenantID: t, CredentialID: id, ProjectID: p, CreatedBy: grantor, CreatedAt: time.Now()}}, options.Update().SetUpsert(true))
	return err
}
func (r *DynamicRepository) DisconnectExternalAPICredential(ctx context.Context, tenantID, credentialID, projectID string) error {
	t, id, err := credentialIDs(tenantID, credentialID)
	if err != nil {
		return err
	}
	p, err := primitive.ObjectIDFromHex(projectID)
	if err != nil {
		return err
	}
	_, err = r.globalCollection("external_api_credential_grants").DeleteOne(ctx, bson.M{"tenantId": t, "credentialId": id, "projectId": p})
	return err
}
func (r *DynamicRepository) PromoteExternalAPICredential(ctx context.Context, tenantID, credentialID string, grantor primitive.ObjectID) error {
	if err := r.EnsureExternalAPICredentialGrantIndexes(ctx); err != nil {
		return err
	}
	return r.WithTransaction(ctx, func(tx mongo.SessionContext) error {
		c, err := r.GetTenantExternalAPICredential(tx, tenantID, credentialID)
		if err != nil {
			return err
		}
		if c.EffectiveScope() == models.ExternalAPICredentialScopeTenant {
			return nil
		}
		if c.EffectiveScope() != models.ExternalAPICredentialScopeProject || c.ProjectID.IsZero() {
			return fmt.Errorf("project credential required")
		}
		if err = r.touchCredentialGrantProject(tx, c.TenantID, c.ProjectID); err != nil {
			return err
		}
		if err = r.upsertExternalCredentialGrant(tx, c.TenantID, c.ID, c.ProjectID, grantor); err != nil {
			return err
		}
		_, err = r.externalAPICredentialsCollection().UpdateOne(tx, bson.M{"_id": c.ID, "tenantId": c.TenantID}, bson.M{"$set": bson.M{"scope": models.ExternalAPICredentialScopeTenant, "updatedAt": time.Now()}, "$unset": bson.M{"projectId": ""}})
		return err
	})
}
