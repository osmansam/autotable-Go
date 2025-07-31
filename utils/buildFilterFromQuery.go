// utils/helpers.go
package utils

import (
	"github.com/gofiber/fiber/v2"
	"github.com/osmansam/autotableGo/models"
	"go.mongodb.org/mongo-driver/bson"
)

func BuildFilterFromQuery(c *fiber.Ctx, container *models.ContainerModel) (bson.M, error) {
    filter := bson.M{}

    for _, f := range container.Fields {
        // skip hashed
        if f.IsHashed {
            continue
        }
        raw := c.Query(f.Name)
        if raw == "" {
            continue
        }

        converted, err := ConvertQueryValueToFieldType(f.Name, f.Type, raw)
        if err != nil {
            return nil, err
        }

        // if it's a map (e.g. {"$gte":…}), merge with existing condition
        if m, ok := converted.(bson.M); ok {
            if existing, exists := filter[f.Name]; exists {
                if emap, ok2 := existing.(bson.M); ok2 {
                    for k, v := range m {
                        emap[k] = v
                    }
                    continue
                }
            }
        }

        filter[f.Name] = converted
    }

    return filter, nil
}
