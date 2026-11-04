// Package bind turns request-binding failures (bad JSON, wrong types, failed `binding:` rules)
// into messages a person can act on, e.g. `"title" must be at most 100 characters` instead of
// "Key: 'CreateAnchorRequest.Title' Error:Field validation for 'Title' failed on the 'max' tag".
package bind

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/xyz-asif/gotodo/internal/pkg/response"
)

var registerOnce sync.Once

// useJSONFieldNames makes validation errors name fields as clients send them ("coverMediaType",
// not "CoverMediaType")
func useJSONFieldNames() {
	registerOnce.Do(func() {
		if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
			v.RegisterTagNameFunc(func(f reflect.StructField) string {
				for _, tag := range []string{"json", "form"} {
					if name := strings.Split(f.Tag.Get(tag), ",")[0]; name != "" && name != "-" {
						return name
					}
				}
				return f.Name
			})
		}
	})
}

func init() { useJSONFieldNames() }

// JSON binds the JSON body into obj. On failure it writes a 400 with a specific message and
// returns false.
func JSON(c *gin.Context, obj interface{}) bool {
	if err := c.ShouldBindJSON(obj); err != nil {
		response.BadRequest(c, Message(err), "INVALID_REQUEST")
		return false
	}
	return true
}

// Message explains a binding error in plain language
func Message(err error) string {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var validationErrs validator.ValidationErrors

	switch {
	case errors.Is(err, io.EOF):
		return "The request body is empty. Please send the data as JSON"
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "The request body is cut off: the JSON isn't complete"
	case errors.As(err, &syntaxErr):
		return fmt.Sprintf("The request body isn't valid JSON (problem near character %d)", syntaxErr.Offset)
	case errors.As(err, &typeErr):
		field := typeErr.Field
		if field == "" {
			return fmt.Sprintf("The request body must be a JSON object, not %s", article(typeErr.Value))
		}
		return fmt.Sprintf(`"%s" must be %s, not %s`, field, describeType(typeErr.Type), article(typeErr.Value))
	case errors.As(err, &validationErrs) && len(validationErrs) > 0:
		return fieldMessage(validationErrs[0])
	}
	return "The request couldn't be read. Please check that it is valid JSON with the expected fields"
}

func fieldMessage(fe validator.FieldError) string {
	name := fe.Field()
	isText := fe.Kind() == reflect.String
	isList := fe.Kind() == reflect.Slice || fe.Kind() == reflect.Array
	unit := ""
	switch {
	case isText:
		unit = " characters"
	case isList:
		unit = " items"
	}
	// Errors inside lists (`dive`) are reported on e.g. "tags[2]"
	switch fe.Tag() {
	case "required":
		return fmt.Sprintf(`"%s" is required`, name)
	case "min":
		if isText || isList {
			return fmt.Sprintf(`"%s" must have at least %s%s`, name, fe.Param(), unit)
		}
		return fmt.Sprintf(`"%s" must be at least %s`, name, fe.Param())
	case "max":
		if isText || isList {
			return fmt.Sprintf(`"%s" can have at most %s%s`, name, fe.Param(), unit)
		}
		return fmt.Sprintf(`"%s" must be at most %s`, name, fe.Param())
	case "len":
		return fmt.Sprintf(`"%s" must have exactly %s%s`, name, fe.Param(), unit)
	case "oneof":
		return fmt.Sprintf(`"%s" must be one of: %s`, name, strings.Join(strings.Fields(fe.Param()), ", "))
	case "email":
		return fmt.Sprintf(`"%s" must be a valid email address`, name)
	case "url", "uri":
		return fmt.Sprintf(`"%s" must be a valid link starting with http:// or https://`, name)
	}
	return fmt.Sprintf(`"%s" has an invalid value`, name)
}

func describeType(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "text"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "a list"
	case reflect.Map, reflect.Struct:
		return "an object"
	case reflect.Ptr:
		return describeType(t.Elem())
	}
	return "a different type"
}

// article turns encoding/json's value kinds ("string", "number", "array", ...) into prose
func article(jsonKind string) string {
	switch {
	case jsonKind == "array" || jsonKind == "object":
		return "an " + jsonKind
	case strings.HasPrefix(jsonKind, "number"):
		return "a number"
	case jsonKind == "bool":
		return "true/false"
	}
	return "a " + jsonKind
}
