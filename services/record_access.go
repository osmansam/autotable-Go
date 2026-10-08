package services

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"

	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type accessIdentityKind string

const (
	accessIdentityCaller accessIdentityKind = "caller"
	accessIdentitySystem accessIdentityKind = "system"
)

type recordAccessContext struct {
	TenantID     string
	ProjectID    string
	Schema       string
	Operation    string
	IdentityKind accessIdentityKind
	UserID       string
	UserRole     string
	UserRoles    []string
	User         map[string]interface{}
}

type readAccessOperation string

const (
	readAccessGetAll    readAccessOperation = "get_all"
	readAccessSelection readAccessOperation = "selection"
	readAccessGetOne    readAccessOperation = "get_one"
	readAccessSearch    readAccessOperation = "search"
	readAccessFilter    readAccessOperation = "filter"
	readAccessPaginated readAccessOperation = "paginated"
	readAccessExport    readAccessOperation = "export"
)

func readAccessRoute(container *models.ContainerModel, operation readAccessOperation) models.RouteSpec {
	if container == nil {
		return models.RouteSpec{}
	}
	switch operation {
	case readAccessGetAll:
		return container.Routes.GetAllDynamicModelItems
	case readAccessSelection:
		return container.Routes.GetItemsForSelection
	case readAccessGetOne:
		return container.Routes.GetDynamicModelItem
	case readAccessSearch:
		return container.Routes.HandleSearchDynamicModelItem
	case readAccessFilter:
		return container.Routes.HandleFilterDynamicModelItem
	case readAccessPaginated:
		return container.Routes.GetAllDynamicModelItemsWithPagination
	case readAccessExport:
		return container.Routes.ExportDynamicModelItems
	default:
		return models.RouteSpec{}
	}
}

func recordAccessDisablesSharedCache(route models.RouteSpec) bool {
	return route.Access != nil
}

func recordReadRequiresIdentityIsolation(container *models.ContainerModel, route models.RouteSpec) bool {
	return recordAccessDisablesSharedCache(route) || (container != nil && container.RowAccess != nil && len(container.RowAccess.Conditions) > 0)
}

func recordAccessReadError(err error, message string) error {
	if err == mongo.ErrNoDocuments {
		return &ServiceError{Status: http.StatusNotFound, Message: message, Err: err}
	}
	return &ServiceError{Status: http.StatusInternalServerError, Message: message, Err: err}
}

func (s *DynamicService) composeReadRecordAccessFilter(
	ctx context.Context,
	container *models.ContainerModel,
	route models.RouteSpec,
	tenantID, projectID, schema, operation string,
	identity AccessIdentityInput,
	base bson.M,
) (bson.M, error) {
	user := map[string]interface{}{
		"id":   identity.UserID,
		"_id":  identity.UserID,
		"role": identity.UserRole,
	}
	var accessCtx recordAccessContext
	if route.Access != nil {
		var err error
		accessCtx, err = s.buildRecordAccessContext(ctx, tenantID, projectID, schema, operation, identity)
		if err != nil {
			return nil, err
		}
		user = cloneAccessMap(accessCtx.User)
		if user == nil {
			user = map[string]interface{}{}
		}
		user["id"] = identity.UserID
		user["_id"] = identity.UserID
		user["role"] = identity.UserRole
		user["roles"] = append([]string(nil), accessCtx.UserRoles...)
	}

	rowAccessFilter, err := utils.GetRowAccessFilter(container, identity.UserRole, user)
	if err != nil {
		return nil, fmt.Errorf("build row access filter: %w", err)
	}
	accessFilter, unrestricted, err := compileRecordAccessFilter(container, route.Access, accessCtx)
	if err != nil {
		return nil, fmt.Errorf("compile record access filter: %w", err)
	}
	if unrestricted {
		accessFilter = nil
	}
	return combineRecordAccessFilters(base, rowAccessFilter, accessFilter), nil
}

func (s *DynamicService) buildMutationRecordAccessContext(
	ctx context.Context,
	route models.RouteSpec,
	tenantID, projectID, schema, operation string,
	identity AccessIdentityInput,
) (recordAccessContext, error) {
	if route.Access == nil {
		return recordAccessContext{}, nil
	}
	return s.buildRecordAccessContext(ctx, tenantID, projectID, schema, operation, identity)
}

func mutationRecordAccessSelector(
	container *models.ContainerModel,
	route models.RouteSpec,
	identity AccessIdentityInput,
	accessCtx recordAccessContext,
	base bson.M,
) (bson.M, error) {
	user := cloneAccessMap(accessCtx.User)
	if user == nil {
		user = map[string]interface{}{}
	}
	user["id"] = identity.UserID
	user["_id"] = identity.UserID
	user["role"] = identity.UserRole
	user["roles"] = append([]string(nil), identity.UserRoles...)

	rowAccessFilter, err := utils.GetRowAccessFilter(container, identity.UserRole, user)
	if err != nil {
		return nil, fmt.Errorf("build row access filter: %w", err)
	}
	accessFilter, unrestricted, err := compileRecordAccessFilter(container, route.Access, accessCtx)
	if err != nil {
		return nil, fmt.Errorf("compile record access filter: %w", err)
	}
	if unrestricted {
		accessFilter = nil
	}
	return combineRecordAccessFilters(base, rowAccessFilter, accessFilter), nil
}

func authorizeFinalUpdateRecord(
	container *models.ContainerModel,
	policy *models.RecordAccessPolicy,
	ctx recordAccessContext,
	record map[string]interface{},
) error {
	allowed, err := recordAccessAllows(container, policy, ctx, record)
	if err != nil {
		return &ServiceError{Status: http.StatusInternalServerError, Message: "Failed to evaluate record access policy", Err: err}
	}
	if !allowed {
		return &ServiceError{Status: http.StatusNotFound, Message: "No item found with specified ID"}
	}
	return nil
}

func updateResultNotFound(result *mongo.UpdateResult) error {
	if result == nil || result.MatchedCount == 0 {
		return &ServiceError{Status: http.StatusNotFound, Message: "No item found with specified ID"}
	}
	return nil
}

func deleteResultNotFound(result *mongo.DeleteResult) error {
	if result == nil || result.DeletedCount == 0 {
		return &ServiceError{Status: http.StatusNotFound, Message: "No item found with specified ID"}
	}
	return nil
}

func (s *DynamicService) buildRecordAccessContext(
	ctx context.Context,
	tenantID, projectID, schema, operation string,
	identity AccessIdentityInput,
) (recordAccessContext, error) {
	identityKind := accessIdentityKind(strings.TrimSpace(identity.IdentityKind))
	if identityKind != accessIdentityCaller && identityKind != accessIdentitySystem {
		return recordAccessContext{}, fmt.Errorf("unsupported access identity kind %q", identity.IdentityKind)
	}

	roles := append([]string(nil), identity.UserRoles...)
	if len(roles) == 0 && identity.AuditUser != nil {
		roles = append(roles, identity.AuditUser.Roles...)
	}
	var user map[string]interface{}
	if identity.UserID != "" {
		if s == nil || s.loadAccessUser == nil {
			return recordAccessContext{}, fmt.Errorf("record access user loader is unavailable")
		}
		loadedUser, err := s.loadAccessUser(ctx, tenantID, projectID, identity.UserID)
		if err != nil {
			return recordAccessContext{}, fmt.Errorf("load record access user: %w", err)
		}
		user = loadedUser
	}

	return recordAccessContext{
		TenantID:     tenantID,
		ProjectID:    projectID,
		Schema:       schema,
		Operation:    operation,
		IdentityKind: identityKind,
		UserID:       identity.UserID,
		UserRole:     identity.UserRole,
		UserRoles:    roles,
		User:         user,
	}, nil
}

func applyCreateRecordAccessAssignments(
	container *models.ContainerModel,
	policy *models.RecordAccessPolicy,
	ctx recordAccessContext,
	record map[string]interface{},
) (map[string]interface{}, error) {
	assigned, err := applyRecordAccessAssignments(container, policy, ctx, record)
	if err != nil {
		return nil, &ServiceError{
			Status:  http.StatusInternalServerError,
			Message: "Failed to resolve trusted record access assignment",
			Err:     err,
		}
	}
	return assigned, nil
}

func authorizePreparedCreateRecords(
	container *models.ContainerModel,
	policy *models.RecordAccessPolicy,
	ctx recordAccessContext,
	records ...map[string]interface{},
) error {
	for index, record := range records {
		allowed, err := recordAccessAllows(container, policy, ctx, record)
		if err != nil {
			return &ServiceError{
				Status:  http.StatusInternalServerError,
				Message: "Failed to evaluate record access policy",
				Err:     err,
			}
		}
		if !allowed {
			return &ServiceError{
				Status:  http.StatusForbidden,
				Message: "Record access policy denied create",
				Data:    map[string]interface{}{"itemIndex": index},
			}
		}
	}
	return nil
}

func buildAccessAuthContext(ctx recordAccessContext) map[string]interface{} {
	user := cloneAccessMap(ctx.User)
	if user == nil {
		user = map[string]interface{}{}
	}
	if ctx.UserID != "" {
		user["id"] = ctx.UserID
		user["_id"] = ctx.UserID
	}
	if ctx.UserRole != "" {
		user["role"] = ctx.UserRole
	}
	if len(ctx.UserRoles) > 0 {
		user["roles"] = append([]string(nil), ctx.UserRoles...)
	}

	return map[string]interface{}{
		"tenant":    map[string]interface{}{"id": ctx.TenantID},
		"project":   map[string]interface{}{"id": ctx.ProjectID},
		"schema":    ctx.Schema,
		"operation": ctx.Operation,
		"identity":  map[string]interface{}{"kind": string(ctx.IdentityKind)},
		"user":      user,
	}
}

func resolveRecordAccessValue(ctx recordAccessContext, value interface{}) (interface{}, bool, error) {
	text, isString := value.(string)
	if !isString {
		return value, true, nil
	}
	path, isExpression, malformed := recordAccessExpressionPath(text)
	if malformed {
		return nil, false, fmt.Errorf("invalid record access context expression %q", text)
	}
	if !isExpression {
		return value, true, nil
	}

	auth := buildAccessAuthContext(ctx)
	resolved, ok := lookupAccessPath(auth, strings.TrimPrefix(path, "auth."))
	return resolved, ok, nil
}

func recordAccessExpressionPath(value string) (path string, expression bool, malformed bool) {
	value = strings.TrimSpace(value)
	mentionsTemplate := strings.Contains(value, "{{") || strings.Contains(value, "}}")
	if !mentionsTemplate {
		return "", false, false
	}
	if !strings.HasPrefix(value, "{{auth.") || !strings.HasSuffix(value, "}}") ||
		strings.Count(value, "{{") != 1 || strings.Count(value, "}}") != 1 {
		return "", false, true
	}
	path = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "{{"), "}}"))
	if path == "" {
		return "", false, true
	}
	return path, true, false
}

func splitAccessPath(path string) []string {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return strings.Split(path, ".")
}

func lookupAccessPath(value interface{}, path string) (interface{}, bool) {
	current := value
	for _, part := range splitAccessPath(path) {
		switch mapped := current.(type) {
		case map[string]interface{}:
			var ok bool
			current, ok = mapped[part]
			if !ok {
				return nil, false
			}
		case bson.M:
			var ok bool
			current, ok = mapped[part]
			if !ok {
				return nil, false
			}
		default:
			return nil, false
		}
	}
	return current, true
}

func applyRecordAccessAssignments(
	container *models.ContainerModel,
	policy *models.RecordAccessPolicy,
	ctx recordAccessContext,
	input map[string]interface{},
) (map[string]interface{}, error) {
	result := cloneAccessMap(input)
	if result == nil {
		result = map[string]interface{}{}
	}
	if policy == nil || len(policy.Assign) == 0 {
		return result, nil
	}

	fieldNames := make([]string, 0, len(policy.Assign))
	for fieldName := range policy.Assign {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Strings(fieldNames)

	for _, fieldName := range fieldNames {
		field, ok := findRecordAccessField(container, fieldName)
		if !ok {
			return nil, fmt.Errorf("record access assignment field %q does not exist", fieldName)
		}
		resolved, available, err := resolveRecordAccessValue(ctx, policy.Assign[fieldName])
		if err != nil {
			return nil, err
		}
		if !available {
			return nil, fmt.Errorf("record access assignment field %q could not resolve its trusted value", fieldName)
		}
		normalized, err := normalizeRecordAccessValue(*field, resolved)
		if err != nil {
			return nil, fmt.Errorf("record access assignment field %q: %w", fieldName, err)
		}
		setAccessPath(result, fieldName, normalized)
	}
	return result, nil
}

func compileRecordAccessFilter(
	container *models.ContainerModel,
	policy *models.RecordAccessPolicy,
	ctx recordAccessContext,
) (bson.M, bool, error) {
	if policy == nil || len(policy.Any) == 0 {
		return nil, true, nil
	}

	predicates := make([]bson.M, 0, len(policy.Any))
	for _, rule := range policy.Any {
		if strings.TrimSpace(rule.Context) != "" {
			left, available, err := resolveRecordAccessValue(ctx, rule.Context)
			if err != nil {
				return nil, false, err
			}
			if !available {
				continue
			}
			right, available, err := resolveRecordAccessValue(ctx, rule.Value)
			if err != nil {
				return nil, false, err
			}
			if !available {
				continue
			}
			if recordAccessValuesMatch(rule.Operator, left, right) {
				return nil, true, nil
			}
			continue
		}

		field, ok := findRecordAccessField(container, rule.Field)
		if !ok {
			return nil, false, fmt.Errorf("record access field %q does not exist", rule.Field)
		}
		right, available, err := resolveRecordAccessValue(ctx, rule.Value)
		if err != nil {
			return nil, false, err
		}
		if !available {
			continue
		}
		normalized, err := normalizeRecordAccessRuleValue(*field, rule.Operator, right)
		if err != nil {
			continue
		}
		predicate, err := recordAccessPredicate(rule.Field, rule.Operator, normalized)
		if err != nil {
			return nil, false, err
		}
		predicates = append(predicates, predicate)
	}

	switch len(predicates) {
	case 0:
		return recordAccessDenyAllFilter(), false, nil
	case 1:
		return predicates[0], false, nil
	default:
		return bson.M{"$or": predicates}, false, nil
	}
}

func recordAccessAllows(
	container *models.ContainerModel,
	policy *models.RecordAccessPolicy,
	ctx recordAccessContext,
	record map[string]interface{},
) (bool, error) {
	if policy == nil || len(policy.Any) == 0 {
		return true, nil
	}

	for _, rule := range policy.Any {
		var left interface{}
		var available bool
		var err error
		if strings.TrimSpace(rule.Context) != "" {
			left, available, err = resolveRecordAccessValue(ctx, rule.Context)
		} else {
			left, available = lookupAccessPath(record, rule.Field)
		}
		if err != nil {
			return false, err
		}
		if !available {
			continue
		}

		right, available, err := resolveRecordAccessValue(ctx, rule.Value)
		if err != nil {
			return false, err
		}
		if !available {
			continue
		}
		if strings.TrimSpace(rule.Field) != "" {
			field, ok := findRecordAccessField(container, rule.Field)
			if !ok {
				return false, fmt.Errorf("record access field %q does not exist", rule.Field)
			}
			left, err = normalizeRecordAccessValue(*field, left)
			if err != nil {
				continue
			}
			right, err = normalizeRecordAccessRuleValue(*field, rule.Operator, right)
			if err != nil {
				continue
			}
		}
		if recordAccessValuesMatch(rule.Operator, left, right) {
			return true, nil
		}
	}
	return false, nil
}

func recordAccessPredicate(fieldName, operator string, value interface{}) (bson.M, error) {
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "eq":
		return bson.M{fieldName: value}, nil
	case "ne":
		return bson.M{fieldName: bson.M{"$ne": value}}, nil
	case "in":
		return bson.M{fieldName: bson.M{"$in": value}}, nil
	case "nin":
		return bson.M{fieldName: bson.M{"$nin": value}}, nil
	default:
		return nil, fmt.Errorf("unsupported record access operator %q", operator)
	}
}

func recordAccessValuesMatch(operator string, left, right interface{}) bool {
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "eq":
		return reflect.DeepEqual(left, right)
	case "ne":
		return !reflect.DeepEqual(left, right)
	case "in":
		return recordAccessSliceContains(right, left)
	case "nin":
		return !recordAccessSliceContains(right, left)
	default:
		return false
	}
}

func recordAccessSliceContains(collection, wanted interface{}) bool {
	value := reflect.ValueOf(collection)
	if !value.IsValid() || (value.Kind() != reflect.Array && value.Kind() != reflect.Slice) {
		return false
	}
	for index := 0; index < value.Len(); index++ {
		if reflect.DeepEqual(value.Index(index).Interface(), wanted) {
			return true
		}
	}
	return false
}

func normalizeRecordAccessRuleValue(field models.Field, operator string, value interface{}) (interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "in", "nin":
		collection := reflect.ValueOf(value)
		if !collection.IsValid() || (collection.Kind() != reflect.Array && collection.Kind() != reflect.Slice) {
			return nil, fmt.Errorf("operator %s requires an array value", operator)
		}
		normalized := make([]interface{}, 0, collection.Len())
		for index := 0; index < collection.Len(); index++ {
			item, err := normalizeRecordAccessValue(field, collection.Index(index).Interface())
			if err != nil {
				return nil, err
			}
			normalized = append(normalized, item)
		}
		return normalized, nil
	default:
		return normalizeRecordAccessValue(field, value)
	}
}

func normalizeRecordAccessValue(field models.Field, value interface{}) (interface{}, error) {
	if !strings.EqualFold(strings.TrimSpace(field.Type), "objectId") {
		return value, nil
	}
	switch typed := value.(type) {
	case primitive.ObjectID:
		return typed, nil
	case string:
		objectID, err := primitive.ObjectIDFromHex(typed)
		if err != nil {
			return nil, fmt.Errorf("value %q is not a valid objectId", typed)
		}
		return objectID, nil
	default:
		return nil, fmt.Errorf("value %#v is not compatible with objectId", value)
	}
}

func findRecordAccessField(container *models.ContainerModel, path string) (*models.Field, bool) {
	if container == nil {
		return nil, false
	}
	parts := splitAccessPath(path)
	fields := container.Fields
	for index, part := range parts {
		found := false
		for fieldIndex := range fields {
			if fields[fieldIndex].Name != part {
				continue
			}
			if index == len(parts)-1 {
				return &fields[fieldIndex], true
			}
			fields = fields[fieldIndex].Children
			found = true
			break
		}
		if !found {
			return nil, false
		}
	}
	return nil, false
}

func setAccessPath(document map[string]interface{}, path string, value interface{}) {
	parts := splitAccessPath(path)
	if len(parts) == 0 {
		return
	}
	current := document
	for _, part := range parts[:len(parts)-1] {
		nested, ok := current[part].(map[string]interface{})
		if !ok {
			nested = map[string]interface{}{}
			current[part] = nested
		}
		current = nested
	}
	current[parts[len(parts)-1]] = value
}

func recordAccessDenyAllFilter() bson.M {
	return bson.M{"_id": bson.M{"$exists": false}}
}

func combineRecordAccessFilters(filters ...bson.M) bson.M {
	nonEmpty := make([]bson.M, 0, len(filters))
	for _, filter := range filters {
		if len(filter) > 0 {
			nonEmpty = append(nonEmpty, filter)
		}
	}
	switch len(nonEmpty) {
	case 0:
		return bson.M{}
	case 1:
		return nonEmpty[0]
	default:
		return bson.M{"$and": nonEmpty}
	}
}

func cloneAccessMap(value map[string]interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(value))
	for key, item := range value {
		cloned[key] = cloneAccessValue(item)
	}
	return cloned
}

func cloneAccessValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneAccessMap(typed)
	case bson.M:
		return cloneAccessMap(map[string]interface{}(typed))
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = cloneAccessValue(item)
		}
		return cloned
	case primitive.A:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = cloneAccessValue(item)
		}
		return cloned
	default:
		return value
	}
}
