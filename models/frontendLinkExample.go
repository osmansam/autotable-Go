package models

// Example usage demonstrating how to create fields with link configurations

// ExampleExternalLink shows how to configure an external website link
// Field value: "google.com" → Link: "https://google.com"
func ExampleExternalLink() Field {
	return Field{
		Name: "website",
		Type: "string",
		Frontend: &Frontend{
			DisplayName:  "Website",
			LinkTemplate: "https://{{value}}",
			LinkType:     "external",
			// LinkLabelField is empty, so frontend uses field value as label
		},
	}
}

// ExampleInternalLink shows how to configure an internal dashboard navigation link
// Uses document _id to navigate to detail page
// Field value: "507f1f77bcf86cd799439011" → Link: "/urunler/507f1f77bcf86cd799439011"
func ExampleInternalLink() Field {
	return Field{
		Name: "productId",
		Type: "objectId",
		Frontend: &Frontend{
			DisplayName:    "Product",
			LinkTemplate:   "/urunler/{{_id}}",
			LinkType:       "internal",
			LinkLabelField: "name", // Use product name as link text instead of ID
		},
	}
}

// ExampleEmailLink shows how to configure a mailto link
// Field value: "user@example.com" → Link: "mailto:user@example.com"
func ExampleEmailLink() Field {
	return Field{
		Name: "email",
		Type: "string",
		Frontend: &Frontend{
			DisplayName:  "Email",
			LinkTemplate: "mailto:{{value}}",
			LinkType:     "email",
		},
	}
}

// ExamplePhoneLink shows how to configure a tel link
// Field value: "+1234567890" → Link: "tel:+1234567890"
func ExamplePhoneLink() Field {
	return Field{
		Name: "phone",
		Type: "string",
		Frontend: &Frontend{
			DisplayName:  "Phone Number",
			LinkTemplate: "tel:{{value}}",
			LinkType:     "phone",
		},
	}
}

// ExampleFileLink shows how to configure a file download link
// Field value: "document.pdf" → Link: "/uploads/document.pdf"
func ExampleFileLink() Field {
	return Field{
		Name: "attachment",
		Type: "string",
		Frontend: &Frontend{
			DisplayName:  "Attachment",
			LinkTemplate: "/uploads/{{value}}",
			LinkType:     "file",
		},
	}
}

// ExampleRowFieldLink shows how to use other row fields in the link template
// Uses {{row.FIELD_NAME}} placeholder to reference other fields in the same document
// Example: If row has {category: "electronics", _id: "123"}
// → Link: "/categories/electronics/products/123"
func ExampleRowFieldLink() Field {
	return Field{
		Name: "productCode",
		Type: "string",
		Frontend: &Frontend{
			DisplayName:    "Product Code",
			LinkTemplate:   "/categories/{{row.category}}/products/{{_id}}",
			LinkType:       "internal",
			LinkLabelField: "productName",
		},
	}
}

// ExampleCompleteContainer shows a complete container with multiple link types
func ExampleCompleteContainer() ContainerModel {
	return ContainerModel{
		SchemaName: "contacts",
		Fields: []Field{
			{
				Name: "name",
				Type: "string",
				Frontend: &Frontend{
					DisplayName: "Name",
				},
			},
			{
				Name: "email",
				Type: "string",
				Frontend: &Frontend{
					DisplayName:  "Email",
					LinkTemplate: "mailto:{{value}}",
					LinkType:     "email",
				},
			},
			{
				Name: "phone",
				Type: "string",
				Frontend: &Frontend{
					DisplayName:  "Phone",
					LinkTemplate: "tel:{{value}}",
					LinkType:     "phone",
				},
			},
			{
				Name: "website",
				Type: "string",
				Frontend: &Frontend{
					DisplayName:  "Website",
					LinkTemplate: "https://{{value}}",
					LinkType:     "external",
				},
			},
			{
				Name: "profilePicture",
				Type: "string",
				Frontend: &Frontend{
					DisplayName:  "Profile Picture",
					LinkTemplate: "/uploads/{{value}}",
					LinkType:     "file",
				},
			},
		},
	}
}
