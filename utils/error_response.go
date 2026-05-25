package utils

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-playground/validator"
	"github.com/gofiber/fiber/v2"
)

type NormalizedErrorResponse struct {
	Status  int
	Message string
	Quiet   bool
}

func NormalizeErrorResponse(err error, genericMessage string) NormalizedErrorResponse {
	if errors.Is(err, context.Canceled) {
		return NormalizedErrorResponse{Quiet: true}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return NormalizedErrorResponse{
			Status:  http.StatusGatewayTimeout,
			Message: "Request timed out",
		}
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		return NormalizedErrorResponse{
			Status:  fiberErr.Code,
			Message: firstNonEmpty(genericMessage, fiberErr.Message),
		}
	}

	var validationErrs validator.ValidationErrors
	if errors.As(err, &validationErrs) {
		return NormalizedErrorResponse{
			Status:  http.StatusBadRequest,
			Message: firstNonEmpty(genericMessage, "Validation failed"),
		}
	}

	if looksLikeClientInputError(err, genericMessage) {
		return NormalizedErrorResponse{
			Status:  http.StatusBadRequest,
			Message: firstNonEmpty(genericMessage, "Invalid request"),
		}
	}

	return NormalizedErrorResponse{
		Status:  http.StatusInternalServerError,
		Message: firstNonEmpty(genericMessage, "Internal server error"),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func looksLikeClientInputError(err error, genericMessage string) bool {
	text := strings.ToLower(strings.TrimSpace(genericMessage + " " + errorString(err)))
	clientInputMarkers := []string{
		"bad request",
		"invalid",
		"missing",
		"required",
		"validation",
		"parse request body",
		"parse the request body",
		"request body",
		"pagination",
		"filter parameter",
		"sort parameter",
		"sort params",
		"schema name",
		"provided id",
		"objectid",
		"limit exceeded",
		"cannot delete",
		"restricted",
	}

	for _, marker := range clientInputMarkers {
		if strings.Contains(text, marker) {
			return true
		}
	}

	return false
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
