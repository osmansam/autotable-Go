package models

import (
	"fmt"
	"strings"
)

// ValidLinkTypes defines the allowed values for Frontend.LinkType
var ValidLinkTypes = []string{
	"external",
	"internal",
	"email",
	"phone",
	"file",
}

// ValidateFrontendLinkConfig validates the link configuration in a Frontend struct
// Returns an error if LinkType is invalid or configuration is inconsistent
func ValidateFrontendLinkConfig(f *Frontend) error {
	if f == nil {
		return nil
	}

	// If LinkType is empty, no validation needed
	if f.LinkType == "" {
		return nil
	}

	// Validate LinkType is one of the allowed values
	isValid := false
	for _, validType := range ValidLinkTypes {
		if f.LinkType == validType {
			isValid = true
			break
		}
	}

	if !isValid {
		return fmt.Errorf(
			"invalid linkType '%s': must be one of [%s]",
			f.LinkType,
			strings.Join(ValidLinkTypes, ", "),
		)
	}

	// Optional: warn if LinkType is set but LinkTemplate is empty
	// This is a soft validation - you may choose to enforce or just warn
	if f.LinkType != "" && f.LinkTemplate == "" {
		// You can choose to return an error here or just log a warning
		// For now, we'll allow it but you can uncomment the line below to enforce
		// return fmt.Errorf("linkTemplate is required when linkType is set")
	}

	return nil
}

// ValidateFieldFrontendConfig validates the frontend configuration for a single field
func ValidateFieldFrontendConfig(field *Field) error {
	if field == nil {
		return nil
	}

	if field.Frontend != nil {
		if err := ValidateFrontendLinkConfig(field.Frontend); err != nil {
			return fmt.Errorf("field '%s': %w", field.Name, err)
		}
	}

	// Recursively validate children fields
	for i := range field.Children {
		if err := ValidateFieldFrontendConfig(&field.Children[i]); err != nil {
			return err
		}
	}

	return nil
}

// ValidateContainerFrontendConfig validates all frontend configurations in a ContainerModel
// This should be called during container creation or update
func ValidateContainerFrontendConfig(container *ContainerModel) error {
	if container == nil {
		return nil
	}

	for i := range container.Fields {
		if err := ValidateFieldFrontendConfig(&container.Fields[i]); err != nil {
			return fmt.Errorf("container '%s': %w", container.SchemaName, err)
		}
	}

	return nil
}

// Example integration function showing how to use validation during container creation
func ValidateAndCreateContainer(container *ContainerModel) error {
	// Validate frontend link configurations
	if err := ValidateContainerFrontendConfig(container); err != nil {
		return fmt.Errorf("frontend validation failed: %w", err)
	}

	// Additional validation logic would go here
	// (e.g., schema name validation, field type validation, etc.)

	// Proceed with container creation
	// db.Collection("containers").InsertOne(ctx, container)

	return nil
}

// Example integration function showing how to use validation during container update
func ValidateAndUpdateContainer(container *ContainerModel) error {
	// Validate frontend link configurations
	if err := ValidateContainerFrontendConfig(container); err != nil {
		return fmt.Errorf("frontend validation failed: %w", err)
	}

	// Additional validation logic would go here

	// Proceed with container update
	// db.Collection("containers").UpdateOne(ctx, filter, update)

	return nil
}
