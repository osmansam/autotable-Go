package controllers

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/configs"
	"github.com/osmansam/autotableGo/models"
	"github.com/osmansam/autotableGo/responses"
	"github.com/osmansam/autotableGo/utils"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// create an item for a given collection
func CreateDynamicModelItem(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")

    // Fetch the associated container model
    var container models.ContainerModel
    err := containerCollection.FindOne(ctx, bson.M{"schemaName": schemaName}).Decode(&container)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status: http.StatusInternalServerError, Message: "Unable to retrieve container model.", Data: &fiber.Map{"data": err.Error()},
        })
    }

    // Check if there is an image field in the container
    hasImageField := false
    for _, field := range container.Fields {
        if field.Type == "image" {
            hasImageField = true
            break
        }
    }

    var itemMap map[string]interface{}
    fileURLs := make(map[string]string)

    if hasImageField {
        // Parse multipart form for image fields
        form, err := c.MultipartForm()
        if err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Error parsing form.", Data: &fiber.Map{"data": err.Error()}})
        }
        itemMap = utils.ProcessFormFields(form.Value)

        // Process image fields
        for fieldName, files := range form.File {
            for _, file := range files {
                tempFilePath := "./temp/" + file.Filename
                if err := c.SaveFile(file, tempFilePath); err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error saving temp file.", Data: &fiber.Map{"data": err.Error()}})
                }
                imageURL, err := utils.UploadToCloudinary(tempFilePath)
                os.Remove(tempFilePath) // Clean up temp file
                if err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error uploading to Cloudinary.", Data: &fiber.Map{"data": err.Error()}})
                }
                fileURLs[fieldName] = imageURL
            }
        }
    } else {
        // Parse the provided item from request body
        if err := c.BodyParser(&itemMap); err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Failed to parse body.", Data: &fiber.Map{"data": err.Error()}})
        }
    }

    // Replace file fields with URLs
    for fieldName, url := range fileURLs {
        itemMap[fieldName] = url
    }

    // Validation
    err = utils.ValidateContainerModel(itemMap, container)
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
            Status: http.StatusBadRequest, Message: "Validation failed.", Data: &fiber.Map{"data": err.Error()},
        })
    }

    // Convert fields that should be ObjectId to ObjectId type
    for _, field := range container.Fields {
        if field.Type == "objectId" {
            if strId, ok := itemMap[field.Name].(string); ok {
                objId, err := primitive.ObjectIDFromHex(strId)
                if err == nil {
                    itemMap[field.Name] = objId
                }
            }
        }
    }

    // Get the associated collection for this schema
    var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)

    // Save the validated item into its associated collection
    result, err := currentCollection.InsertOne(ctx, itemMap)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status: http.StatusInternalServerError, Message: "Failed to save item.", Data: &fiber.Map{"data": err.Error()},
        })
    }

    // Successfully saved the item
    return c.Status(http.StatusCreated).JSON(responses.GeneralResponse{
        Status: http.StatusCreated, Message: "Item successfully created.", Data: &fiber.Map{"data": result},
    })
}

//get all item for given collection
func GetAllDynamicModelItems(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	
	// Fetching the schema name from the query params
	schemaName := c.Query("schemaName")
	//check if schema is in containers
	if err := utils.IsSchemaInContainers(ctx, containerCollection, schemaName); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Schema name is not valid. Please provide a valid schema name.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)

	// Fetching all items from the specified collection
	results, err := currentCollection.Find(ctx, bson.M{})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to fetch items from the specified collection in the database.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}
	defer results.Close(ctx)

	// Placeholder to hold all items
	var items []map[string]interface{}

	// Iterating over each item and appending to the items slice
	for results.Next(ctx) {
		var singleItem map[string]interface{}
		if err = results.Decode(&singleItem); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "Failed to decode an item from the specified collection.",
				Data:    &fiber.Map{"data": err.Error()},
			})
		}
		items = append(items, singleItem)
	}

	// Successfully fetched all items
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Successfully fetched all items from the specified collection.",
		Data:    &fiber.Map{"data": items},
	})
}
//delete an item from the collection
func DeleteDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetching the schema name from the query params
	schemaName := c.Query("schemaName")
	//check if schema is in containers
	if err := utils.IsSchemaInContainers(ctx, containerCollection, schemaName); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Schema name is not valid. Please provide a valid schema name.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)

	// Attempting to convert the ID from string to ObjectID
	deleteIdStr := c.Params("id")
	deleteId, err := primitive.ObjectIDFromHex(deleteIdStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Provided ID is not in the valid format.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Attempting to delete the item with the given ID from the specified collection
	result, err := currentCollection.DeleteOne(ctx, bson.M{"_id": deleteId})
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Failed to delete the item from the specified collection.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Successfully deleted the item
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Item successfully deleted from the specified collection.",
		Data:    &fiber.Map{"data": result},
	})
}
//update an item in the collection
func UpdateDynamicModelItem(c *fiber.Ctx) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    schemaName := c.Query("schemaName")
    updateIdStr := c.Params("id")
    updateId, err := primitive.ObjectIDFromHex(updateIdStr)
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
            Status:  http.StatusBadRequest,
            Message: "Provided ID is not in the valid format.",
            Data:    &fiber.Map{"data": err.Error()},
        })
    }

    // Fetch the container model
    var container models.ContainerModel
    err = containerCollection.FindOne(ctx, bson.M{"schemaName": schemaName}).Decode(&container)
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Unable to retrieve the container model from the database.",
            Data:    &fiber.Map{"data": err.Error()},
        })
    }

    // Check if there is an image field in the container
    hasImageField := false
    for _, field := range container.Fields {
        if field.Type == "image" {
            hasImageField = true
            break
        }
    }

    var updatedItemMap map[string]interface{}
    fileURLs := make(map[string]string)

    if hasImageField  {
        // Handle multipart form data
        form, err := c.MultipartForm()
        if err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Error in multipart form", Data: &fiber.Map{"data": err.Error()}})
        }
        updatedItemMap = utils.ProcessFormFields(form.Value)

        // Process image fields
        for fieldName, files := range form.File {
            for _, file := range files {
                tempFilePath := "./temp/" + file.Filename
                if err := c.SaveFile(file, tempFilePath); err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error saving temp file", Data: &fiber.Map{"data": err.Error()}})
                }
                imageURL, err := utils.UploadToCloudinary(tempFilePath)
                os.Remove(tempFilePath)
                if err != nil {
                    return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Error uploading to Cloudinary", Data: &fiber.Map{"data": err.Error()}})
                }
                fileURLs[fieldName] = imageURL
            }
        }
    } else {
        // Handle JSON body
        if err := c.BodyParser(&updatedItemMap); err != nil {
            return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Failed to parse body", Data: &fiber.Map{"data": err.Error()}})
        }
    }

    // Replace file fields with URLs
    for fieldName, url := range fileURLs {
        updatedItemMap[fieldName] = url
    }

    // Validation
    err = utils.ValidateContainerModel(updatedItemMap, container)
    if err != nil {
        return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{Status: http.StatusBadRequest, Message: "Validation failed", Data: &fiber.Map{"data": err.Error()}})
    }

    // Update in collection
    var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)
    updateResult, err := currentCollection.UpdateOne(ctx, bson.M{"_id": updateId}, bson.M{"$set": updatedItemMap})
    if err != nil {
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{Status: http.StatusInternalServerError, Message: "Failed to update item", Data: &fiber.Map{"data": err.Error()}})
    }

    if updateResult.MatchedCount == 0 {
        return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{Status: http.StatusNotFound, Message: "No item found with specified ID", Data: nil})
    }

    return c.Status(http.StatusOK).JSON(responses.GeneralResponse{Status: http.StatusOK, Message: "Item successfully updated", Data: &fiber.Map{"data": updateResult}})
}

// get an item from the database
func GetDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")

	// Check if the schema is present in containers
	if err := utils.IsSchemaInContainers(ctx, containerCollection, schemaName); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Schema name is not valid. Please provide a valid schema name.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)
	
	// Get the ID of the item to be fetched from the params and attempt to convert it to ObjectID
	getIdStr := c.Params("id")
	getId, err := primitive.ObjectIDFromHex(getIdStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Provided ID is not in the valid format.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Get the item from the collection
	var result bson.M
	if err := currentCollection.FindOne(ctx, bson.M{"_id": getId}).Decode(&result); err != nil {
		return c.Status(http.StatusNotFound).JSON(responses.GeneralResponse{
			Status:  http.StatusNotFound,
			Message: "No item found with the specified ID.",
			Data:    nil,
		})
	}

	// Successfully get the item
	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Item successfully fetched.",
		Data:    &fiber.Map{"data": result},
	})
}
// handleSearch for a given collection
func HandleSearchDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	searchKey := c.Query("searchKey")

	// Fetch the associated container model from DB based on the schema name
	var container models.ContainerModel
	err := containerCollection.FindOne(ctx, bson.M{"schemaName": schemaName}).Decode(&container)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Unable to retrieve the container model from the database.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Define the regex pattern to match anywhere in the string
	pattern := ".*" + searchKey + ".*"
	regex := primitive.Regex{Pattern: pattern, Options: "i"} // "i" is for case-insensitive

	// Build the query filter
	var orQueries []bson.M
	for _, field := range container.Fields {
		if field.Type == "string" {
			orQueries = append(orQueries, bson.M{field.Name: regex})
		}
	}
	filter := bson.M{"$or": orQueries}

	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)
	results, err := currentCollection.Find(ctx, filter)
	if err != nil {
		return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
			Status:  http.StatusInternalServerError,
			Message: "Error fetching search results.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Reading from the db
	var items []map[string]interface{}
	defer results.Close(ctx)
	for results.Next(ctx) {
		var item map[string]interface{}
		if err = results.Decode(&item); err != nil {
			return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
				Status:  http.StatusInternalServerError,
				Message: "Error decoding search result.",
				Data:    &fiber.Map{"data": err.Error()},
			})
		}
		items = append(items, item)
	}

	return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
		Status:  http.StatusOK,
		Message: "Search results fetched successfully.",
		Data:    &fiber.Map{"data": items},
	})
}
//get all item for given collection
func GetPipeline(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipelineInput := models.PipelineStageInput{
        Match:     c.Query("match"),
        Lookup:    c.Query("lookup"),
        Unwind:    c.Query("unwind"),
        Group:     c.Query("group"),
        Sort:      c.Query("sort"),
        AddFields: c.Query("addFields"),
        Limit:     c.Query("limit"),
        Skip:      c.Query("skip"),
        Facet:     c.Query("facet"),
        Merge:     c.Query("merge"),
        Out:       c.Query("out"),
    }
		// Fetching the schema name from the query params
	schemaName := c.Query("schemaName")
	//check if schema is in containers
	if err := utils.IsSchemaInContainers(ctx, containerCollection, schemaName); err != nil {
		return c.Status(http.StatusBadRequest).JSON(responses.GeneralResponse{
			Status:  http.StatusBadRequest,
			Message: "Schema name is not valid. Please provide a valid schema name.",
			Data:    &fiber.Map{"data": err.Error()},
		})
	}

	// Using the schema name to determine the appropriate collection
	var currentCollection *mongo.Collection = configs.GetCollection(configs.DB, schemaName)
	
items, err := utils.ExecuteDynamicPipeline(ctx, currentCollection, pipelineInput)
    if err != nil {
        // Handle error
        return c.Status(http.StatusInternalServerError).JSON(responses.GeneralResponse{
            Status:  http.StatusInternalServerError,
            Message: "Failed to execute dynamic pipeline.",
            Data:    &fiber.Map{"error": err.Error()},
        })
    }

    // Return the results
    return c.Status(http.StatusOK).JSON(responses.GeneralResponse{
        Status:  http.StatusOK,
        Message: "Successfully fetched items using dynamic pipeline.",
        Data:    &fiber.Map{"data": items},
    })
}