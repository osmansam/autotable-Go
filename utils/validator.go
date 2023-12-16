package utils

// import (
// 	"fmt"
// 	"reflect"
// 	"regexp"
// 	"strings"

// 	"github.com/go-playground/validator"
// 	"github.com/osmansam/autotableGo/models"
// )

// type validationInfo struct {
//     rules    string

// }
// func ParseTag(tag string) validationInfo {
//     var rules []string

//     // Regular expression to match validation rules
//     ruleRegex := regexp.MustCompile(`([a-zA-Z]+)=("[^"]+"|\d+)`)
//     ruleMatches := ruleRegex.FindAllStringSubmatch(tag, -1)

//     for _, match := range ruleMatches {
//         if len(match) > 2 {
//             rules = append(rules, match[1]+"="+strings.Trim(match[2], "\""))
//         }
//     }

//     return validationInfo{rules: strings.Join(rules, ",")}
// }

// func ValidateField(field models.Field, value interface{}, validate *validator.Validate) error {
//     tagInfo := ParseTag(field.Tag)

//     tempStruct := struct {
//         Value interface{} `validate:""`
//     }{
//         Value: value,
//     }

//     fieldType := reflect.TypeOf(tempStruct)
//     fieldVal := fieldType.Field(0)
//     fieldTag := reflect.StructTag(`validate:"` + tagInfo.rules + `"`)
//     fieldVal.Tag = fieldTag

// 	fmt.Println(tempStruct)
//     if err := validate.Struct(tempStruct); err != nil {
//         for _, fe := range err.(validator.ValidationErrors) {
//             // Return the default error message
//             return fmt.Errorf("%s failed '%s' validation", fe.Field(), fe.Tag())
//         }
//     }

//     return nil
// }

// func ValidateContainerModel(item map[string]interface{}, container models.ContainerModel) error {
//     validate := validator.New()

//     for _, field := range container.Fields {
//         fieldValue, exists := item[field.Name]
//         if !exists {
//             return fmt.Errorf("Field %s is missing", field.Name)
//         }

//         if err := ValidateField(field, fieldValue, validate); err != nil {
//             return err
//         }
//     }

//     return nil
// }
