package collaborationmigration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/services"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sort"
)

type Report struct {
	Hash      string   `json:"hash"`
	Conflicts []string `json:"conflicts"`
	Users     int      `json:"users"`
	Projects  int      `json:"projects"`
}
type state struct {
	Users              []models.User
	Tenants            []models.Tenant
	TenantMemberships  []models.TenantMembership
	ProjectMemberships []models.ProjectMembership
	Projects           []models.Project
}

func readState(ctx context.Context, db *mongo.Database) (state, error) {
	s := state{}
	for _, entry := range []struct {
		name   string
		target interface{}
	}{
		{"users", &s.Users}, {"tenants", &s.Tenants}, {"tenant_memberships", &s.TenantMemberships}, {"project_memberships", &s.ProjectMemberships}, {"projects", &s.Projects},
	} {
		cursor, err := db.Collection(entry.name).Find(ctx, bson.M{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.M{"password": 0}))
		if err != nil {
			return s, err
		}
		err = cursor.All(ctx, entry.target)
		_ = cursor.Close(ctx)
		if err != nil {
			return s, err
		}
	}
	return s, nil
}
func Preflight(ctx context.Context, db *mongo.Database) (Report, error) {
	s, err := readState(ctx, db)
	if err != nil {
		return Report{}, err
	}
	report := Report{Users: len(s.Users), Projects: len(s.Projects), Conflicts: []string{}}
	encoded, err := json.Marshal(s)
	if err != nil {
		return report, err
	}
	hash := sha256.Sum256(encoded)
	report.Hash = hex.EncodeToString(hash[:])
	seen := map[string]bool{}
	activeUsers := map[string]bool{}
	for _, user := range s.Users {
		email := models.NormalizeManagementEmail(user.Email)
		if email == "" {
			report.Conflicts = append(report.Conflicts, "management account has no email: "+user.ID.Hex())
		}
		if seen[email] {
			report.Conflicts = append(report.Conflicts, "duplicate normalized management email: "+email)
		}
		seen[email] = true
		activeUsers[user.ID.Hex()] = user.IsActive
	}
	activeTenants := map[string]bool{}
	for _, tenant := range s.Tenants {
		activeTenants[tenant.ID.Hex()] = tenant.IsActive
	}
	seen = map[string]bool{}
	eligible := map[string]bool{}
	for _, m := range s.TenantMemberships {
		key := m.TenantID.Hex() + ":" + m.UserID.Hex()
		if seen[key] {
			report.Conflicts = append(report.Conflicts, "duplicate tenant membership: "+key)
		}
		seen[key] = true
		_, roleErr := services.CurrentManagementSessionRoles(models.ManagementMembershipSnapshot{UserActive: activeUsers[m.UserID.Hex()], TenantActive: activeTenants[m.TenantID.Hex()], TenantMembership: &m}, false)
		eligible[key] = roleErr == nil
	}
	seen = map[string]bool{}
	admins := map[string]int{}
	for _, m := range s.ProjectMemberships {
		key := m.TenantID.Hex() + ":" + m.ProjectID.Hex() + ":" + m.UserID.Hex()
		if seen[key] {
			report.Conflicts = append(report.Conflicts, "duplicate project membership: "+key)
		}
		seen[key] = true
		if m.Status == models.MembershipStatusActive && eligible[m.TenantID.Hex()+":"+m.UserID.Hex()] {
			for _, role := range m.Roles {
				if role == models.ProjectRoleAdmin {
					admins[m.TenantID.Hex()+":"+m.ProjectID.Hex()]++
					break
				}
			}
		}
	}
	for _, project := range s.Projects {
		if project.IsActive && admins[project.TenantID.Hex()+":"+project.ID.Hex()] == 0 {
			report.Conflicts = append(report.Conflicts, fmt.Sprintf("active project has no usable admin: %s", project.ID.Hex()))
		}
	}
	sort.Strings(report.Conflicts)
	return report, nil
}
