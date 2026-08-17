package controllers

import (
	"bytes"
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/requests"
	"github.com/osmansam/autotableGo/services"
	"github.com/osmansam/autotableGo/utils"
)

type dynamicArrayHandlerOperation string

const (
	dynamicArrayAdd     dynamicArrayHandlerOperation = "add"
	dynamicArrayUpdate  dynamicArrayHandlerOperation = "update"
	dynamicArrayDelete  dynamicArrayHandlerOperation = "delete"
	dynamicArrayReorder dynamicArrayHandlerOperation = "reorder"
)

func handleDynamicArrayMutation(c *fiber.Ctx, operation dynamicArrayHandlerOperation) error {
	ctx, cancel := utils.RequestContextWithTimeout(c, 10*time.Second)
	defer cancel()

	tenantID, projectID, err := getProjectContext(c)
	if err != nil {
		return utils.SendErrorResponse(c, err, err.Error())
	}
	userID, _ := c.Locals("userID").(string)
	idempotencyKey, shouldContinue, err := beginDynamicIdempotency(ctx, c, tenantID, projectID, userID)
	if !shouldContinue {
		return err
	}

	input := services.DynamicArrayMutationInput{
		TenantID:   tenantID,
		ProjectID:  projectID,
		Schema:     c.Params("schema"),
		ParentID:   c.Params("id"),
		ArrayField: c.Params("field"),
		UserID:     userID,
		User:       utils.GetUserFromContext(c),
	}
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		input.Container, _ = storedContainer.(*models.ContainerModel)
	}

	service := services.NewDynamicService()
	var result *services.DynamicArrayMutationResult
	switch operation {
	case dynamicArrayAdd:
		input.Request, err = requests.ParseArrayRowMutation(bytes.NewReader(c.Body()), requests.ArrayMutationAdd)
		if err == nil {
			result, err = service.AddArrayRow(ctx, input)
		}
	case dynamicArrayUpdate, dynamicArrayDelete:
		parserOperation := requests.ArrayMutationUpdate
		if operation == dynamicArrayDelete {
			parserOperation = requests.ArrayMutationDelete
		}
		input.Request, err = requests.ParseArrayRowMutation(bytes.NewReader(c.Body()), parserOperation)
		if err == nil {
			input.RowIdentity, err = url.PathUnescape(c.Params("rowIdentity"))
		}
		if err == nil && operation == dynamicArrayUpdate {
			result, err = service.UpdateArrayRow(ctx, input)
		}
		if err == nil && operation == dynamicArrayDelete {
			result, err = service.DeleteArrayRow(ctx, input)
		}
	case dynamicArrayReorder:
		input.Reorder, err = requests.ParseArrayReorder(bytes.NewReader(c.Body()))
		if err == nil {
			result, err = service.ReorderArrayRows(ctx, input)
		}
	}
	if err != nil {
		if result == nil {
			if _, ok := err.(*services.ServiceError); !ok {
				err = &services.ServiceError{Status: http.StatusBadRequest, Message: err.Error(), Err: err}
			}
		}
		return sendDynamicServiceError(ctx, c, idempotencyKey, err, "Failed to mutate embedded array")
	}
	return sendIdempotentResponse(context.Background(), c, idempotencyKey, http.StatusOK, "Embedded array updated successfully", result)
}

func AddDynamicArrayRow(c *fiber.Ctx) error {
	return handleDynamicArrayMutation(c, dynamicArrayAdd)
}

func UpdateDynamicArrayRow(c *fiber.Ctx) error {
	return handleDynamicArrayMutation(c, dynamicArrayUpdate)
}

func DeleteDynamicArrayRow(c *fiber.Ctx) error {
	return handleDynamicArrayMutation(c, dynamicArrayDelete)
}

func ReorderDynamicArrayRows(c *fiber.Ctx) error {
	return handleDynamicArrayMutation(c, dynamicArrayReorder)
}
