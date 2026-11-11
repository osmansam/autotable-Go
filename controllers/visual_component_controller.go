package controllers

import (
	"context"
	"errors"
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type VisualComponentStore interface {
	List(context.Context, string, string) ([]models.VisualComponentDesign, error)
	Create(context.Context, string, string, models.VisualComponentDesign) (models.VisualComponentDesign, error)
	Update(context.Context, string, string, models.VisualComponentDesign) (models.VisualComponentDesign, error)
}
type VisualComponentController struct{ Store VisualComponentStore }

var VisualComponents = VisualComponentController{Store: repositories.NewVisualComponentRepository()}

func (h VisualComponentController) List(c *fiber.Ctx) error {
	tenant, project, err := getPageProjectContext(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"message": "Project context is required"})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	values, err := h.Store.List(ctx, tenant, project)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"message": "Visual designs could not be loaded"})
	}
	return c.JSON(values)
}
func (h VisualComponentController) Save(c *fiber.Ctx) error {
	tenant, project, err := getPageProjectContext(c)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"message": "Project context is required"})
	}
	var input models.VisualComponentDesign
	if err = c.BodyParser(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"message": "Invalid visual design request"})
	}
	if err = models.ValidateVisualComponentDesign(&input); err != nil {
		return c.Status(400).JSON(fiber.Map{"message": err.Error()})
	}
	ctx, cancel := context.WithTimeout(c.UserContext(), 10*time.Second)
	defer cancel()
	var saved models.VisualComponentDesign
	if c.Method() == fiber.MethodPost {
		saved, err = h.Store.Create(ctx, tenant, project, input)
	} else {
		input.ID, err = primitive.ObjectIDFromHex(c.Params("id"))
		if err != nil || input.Revision < 1 {
			return c.Status(400).JSON(fiber.Map{"message": "Valid design ID and revision are required"})
		}
		saved, err = h.Store.Update(ctx, tenant, project, input)
	}
	if errors.Is(err, repositories.ErrVisualComponentConflict) {
		return c.Status(409).JSON(fiber.Map{"message": err.Error()})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"message": "Visual design could not be saved"})
	}
	if c.Method() == fiber.MethodPost {
		return c.Status(201).JSON(saved)
	}
	return c.JSON(saved)
}
