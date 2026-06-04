package controllers

import (
	"context"
	"net/http"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/repositories"
	"github.com/osmansam/autotableGo/utils"
	"github.com/osmansam/autotableGo/ws"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var notificationRepository = repositories.NewDynamicRepository()

type markNotificationReadRequest struct {
	ID  string   `json:"id"`
	IDs []string `json:"ids"`
}

func GetNotifications(c *fiber.Ctx) error {
	return queryNotifications(c, false)
}

func GetUnreadNotifications(c *fiber.Ctx) error {
	return queryNotifications(c, true)
}

func MarkNotificationRead(c *fiber.Ctx) error {
	if c.Params("id") == "" {
		return markNotificationReadFromBody(c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, userID, roles, err := notificationRequestContext(c)
	if err != nil {
		return err
	}
	id, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid notification id"})
	}

	filter := notificationVisibleFilter(tenantID, projectID, userID, roles, false)
	filter["_id"] = id
	result, err := notificationRepository.MarkNotificationsRead(ctx, tenantID, projectID, userID, filter)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if result.MatchedCount == 0 {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Notification not found"})
	}
	ws.EmitNotificationChanged(userID, tenantID, projectID)
	return c.JSON(fiber.Map{"matched": result.MatchedCount, "modified": result.ModifiedCount})
}

func markNotificationReadFromBody(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, userID, roles, err := notificationRequestContext(c)
	if err != nil {
		return err
	}
	var input markNotificationReadRequest
	if err := c.BodyParser(&input); err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	ids, err := notificationObjectIDs(input)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	filter := notificationVisibleFilter(tenantID, projectID, userID, roles, false)
	filter["_id"] = bson.M{"$in": ids}
	result, err := notificationRepository.MarkNotificationsRead(ctx, tenantID, projectID, userID, filter)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	ws.EmitNotificationChanged(userID, tenantID, projectID)
	return c.JSON(fiber.Map{"matched": result.MatchedCount, "modified": result.ModifiedCount})
}

func MarkAllNotificationsRead(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, userID, roles, err := notificationRequestContext(c)
	if err != nil {
		return err
	}
	filter := notificationVisibleFilter(tenantID, projectID, userID, roles, true)
	result, err := notificationRepository.MarkNotificationsRead(ctx, tenantID, projectID, userID, filter)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	ws.EmitNotificationChanged(userID, tenantID, projectID)
	return c.JSON(fiber.Map{"matched": result.MatchedCount, "modified": result.ModifiedCount})
}

func DeleteNotification(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, userID, roles, err := notificationRequestContext(c)
	if err != nil {
		return err
	}
	id, err := primitive.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": "Invalid notification id"})
	}

	filter := notificationVisibleFilter(tenantID, projectID, userID, roles, false)
	filter["_id"] = id
	items, err := notificationRepository.QueryNotifications(ctx, tenantID, projectID, filter, options.Find().SetLimit(1), &utils.Pager{})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if len(items) == 0 {
		return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Notification not found"})
	}
	result, err := notificationRepository.DeleteNotificationForUser(ctx, tenantID, projectID, id, userID)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	ws.EmitNotificationChanged(userID, tenantID, projectID)
	return c.JSON(fiber.Map{"matched": result.MatchedCount, "modified": result.ModifiedCount})
}

func queryNotifications(c *fiber.Ctx, unreadOnly bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenantID, projectID, userID, roles, err := notificationRequestContext(c)
	if err != nil {
		return err
	}
	pager, err := utils.ParsePager(c)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	sort, err := utils.ParseSort(c)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if sort == nil {
		sort = bson.D{{Key: "createdAt", Value: -1}}
	}

	filter := notificationVisibleFilter(tenantID, projectID, userID, roles, unreadOnly)
	if event := c.Query("event"); event != "" {
		filter["event"] = event
	}
	if schemaName := c.Query("schemaName"); schemaName != "" {
		filter["schemaName"] = schemaName
	}

	if err := notificationRepository.EnsureNotificationIndexes(ctx, tenantID, projectID); err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	items, err := notificationRepository.QueryNotifications(ctx, tenantID, projectID, filter, utils.BuildFindOptions(sort, pager), &pager)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !pager.Enabled {
		return c.JSON(items)
	}
	return c.JSON(fiber.Map{
		"items":      items,
		"totalItems": pager.TotalItems,
		"totalPages": pager.TotalPages,
		"page":       pager.Page,
		"limit":      pager.Limit,
	})
}

func notificationRequestContext(c *fiber.Ctx) (string, string, string, []string, error) {
	tenantID, _ := c.Locals("tenantID").(string)
	projectID, _ := c.Locals("projectID").(string)
	userID, _ := c.Locals("userID").(string)
	if userID == "" {
		userID, _ = c.Locals("tenantUserID").(string)
	}
	if tenantID == "" || projectID == "" || userID == "" {
		return "", "", "", nil, c.Status(http.StatusUnauthorized).JSON(fiber.Map{"error": "Authentication required"})
	}
	return tenantID, projectID, userID, notificationUserRoles(c), nil
}

func notificationObjectIDs(input markNotificationReadRequest) ([]primitive.ObjectID, error) {
	rawIDs := input.IDs
	if input.ID != "" {
		rawIDs = append(rawIDs, input.ID)
	}
	if len(rawIDs) == 0 {
		return nil, fiber.NewError(http.StatusBadRequest, "id or ids is required")
	}
	ids := make([]primitive.ObjectID, 0, len(rawIDs))
	seen := map[primitive.ObjectID]struct{}{}
	for _, rawID := range rawIDs {
		id, err := primitive.ObjectIDFromHex(rawID)
		if err != nil {
			return nil, fiber.NewError(http.StatusBadRequest, "Invalid notification id")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, nil
}

func notificationUserRoles(c *fiber.Ctx) []string {
	seen := map[string]struct{}{}
	var roles []string
	if role, _ := c.Locals("userRole").(string); role != "" {
		seen[role] = struct{}{}
		roles = append(roles, role)
	}
	if localRoles, ok := c.Locals("roles").([]string); ok {
		for _, role := range localRoles {
			if role == "" {
				continue
			}
			if _, exists := seen[role]; exists {
				continue
			}
			seen[role] = struct{}{}
			roles = append(roles, role)
		}
	}
	return roles
}

func notificationVisibleFilter(tenantID, projectID, userID string, roles []string, unreadOnly bool) bson.M {
	filter := bson.M{
		"tenantId":  tenantID,
		"projectId": projectID,
		"isActive":  true,
		"$and": []bson.M{
			notificationRecipientFilter(userID, roles),
			notificationExpiryFilter(),
			bson.M{"deletedBy": bson.M{"$ne": userID}},
		},
	}
	if unreadOnly {
		filter["seenBy"] = bson.M{"$ne": userID}
	}
	return filter
}

func notificationRecipientFilter(userID string, roles []string) bson.M {
	or := []bson.M{
		{"selectedUsers": userID},
		{"selectedUsers": bson.M{"$size": 0}, "selectedRoles": bson.M{"$size": 0}},
		{"selectedUsers": bson.M{"$size": 0}, "selectedRoles": bson.M{"$exists": false}},
		{"selectedUsers": bson.M{"$exists": false}, "selectedRoles": bson.M{"$size": 0}},
		{"selectedUsers": bson.M{"$exists": false}, "selectedRoles": bson.M{"$exists": false}},
	}
	if len(roles) > 0 {
		or = append(or, bson.M{"selectedRoles": bson.M{"$in": roles}})
	}
	return bson.M{"$or": or}
}

func notificationExpiryFilter() bson.M {
	now := time.Now()
	return bson.M{"$or": []bson.M{
		{"expireAt": bson.M{"$exists": false}},
		{"expireAt": nil},
		{"expireAt": bson.M{"$gt": now}},
	}}
}
