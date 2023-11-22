package controllers

import (
	"context"
	"encoding/json"
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
	var container *models.ContainerModel
	var err error

	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		// Fetch container model if not available in context
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
		}
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
   err = utils.ValidateContainerModel(itemMap, *container)
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
// get all items for a given collection
func GetAllDynamicModelItems(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	if schemaName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schema name is required"})
	}

  	var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        // Use the container model from the context if available
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        // Fetch container model if not available in context
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
        }
    }

	redisKey, shouldCache := utils.GenerateRedisKey("GetAllDynamicModelItems", schemaName, container)
	if shouldCache {
		cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
		
		if err == nil {
			var items []map[string]interface{}
			if err := json.Unmarshal([]byte(cachedData), &items); err == nil {
				// Data fetched from cache
				return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": items, "source": "cache"})
			}
		}

		// If not found in cache, fetch from database
		currentCollection := configs.GetCollection(configs.DB, schemaName)
		results, err := currentCollection.Find(ctx, bson.M{})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch items from the database"})
		}
		defer results.Close(ctx)

		var items []map[string]interface{}
		for results.Next(ctx) {
			var singleItem map[string]interface{}
			if err = results.Decode(&singleItem); err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to decode an item"})
			}
			items = append(items, singleItem)
		}

		// Cache the data fetched from database
		dataToCache, _ := json.Marshal(items)
		configs.RedisClient.Set(ctx, redisKey, dataToCache, 24*time.Hour)

		// Data fetched from database
		return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": items, "source": "database"})
	}

	// If caching is not enabled, fetch from database
	currentCollection := configs.GetCollection(configs.DB, schemaName)
	results, err := currentCollection.Find(ctx, bson.M{})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch items from the database"})
	}
	defer results.Close(ctx)

	var items []map[string]interface{}
	for results.Next(ctx) {
		var singleItem map[string]interface{}
		if err = results.Decode(&singleItem); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to decode an item"})
		}
		items = append(items, singleItem)
	}

	// Data fetched from database
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": items, "source": "database"})
}

//delete an item from the collection
func DeleteDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Fetching the schema name from the query params
	schemaName := c.Query("schemaName")

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

    // Fetch the associated container model
	var container *models.ContainerModel
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		// Fetch container model if not available in context
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
		}
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
    err = utils.ValidateContainerModel(updatedItemMap, *container)
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
    if schemaName == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Schema name is required"})
    }

    getIdStr := c.Params("id")
    getId, err := primitive.ObjectIDFromHex(getIdStr)
    if err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid ID format"})
    }

   	var container *models.ContainerModel
    if storedContainer := c.Locals("containerModel"); storedContainer != nil {
        // Use the container model from the context if available
        container, _ = storedContainer.(*models.ContainerModel)
    } else {
        // Fetch container model if not available in context
        var err error
        container, err = utils.GetContainerModel(schemaName)
        if err != nil {
            return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
        }
    }

    redisKey, shouldCache := utils.GenerateRedisKey("GetDynamicModelItem", schemaName, container, getIdStr)
    if shouldCache {
        cachedData, err := configs.RedisClient.Get(ctx, redisKey).Result()
        if err == nil {
            var item bson.M
            if err := json.Unmarshal([]byte(cachedData), &item); err == nil {
                // Data fetched from cache
                return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": item, "source": "cache"})
            }
        }
    }

    currentCollection := configs.GetCollection(configs.DB, schemaName)
    var result bson.M
    if err := currentCollection.FindOne(ctx, bson.M{"_id": getId}).Decode(&result); err != nil {
        return c.Status(http.StatusNotFound).JSON(fiber.Map{"error": "Item not found"})
    }

    if shouldCache {
        dataToCache, _ := json.Marshal(result)
        configs.RedisClient.Set(ctx, redisKey, dataToCache, 24*time.Hour)
    }

    return c.Status(fiber.StatusOK).JSON(fiber.Map{"data": result, "source": "database"})
}
// handleSearch for a given collection
func HandleSearchDynamicModelItem(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	schemaName := c.Query("schemaName")
	searchKey := c.Query("searchKey")

	    // Fetch the associated container model
	var container *models.ContainerModel
	var err error
	if storedContainer := c.Locals("containerModel"); storedContainer != nil {
		// Use the container model from the context if available
		container, _ = storedContainer.(*models.ContainerModel)
	} else {
		// Fetch container model if not available in context
		container, err = utils.GetContainerModel(schemaName)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch container model"})
		}
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