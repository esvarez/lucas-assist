package api

import (
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

// validate is constructed once at package init and reused across requests
// — matches the "clients constructed once" convention for the
// DynamoDB/OpenAI clients (AGENTS.MD). Field names in errors come from the
// json tag (e.g. "user_id") rather than the Go field name (e.g. "UserID"),
// since that's what the caller actually sent.
var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New()
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return v
}

// validationErrorResponse is the 400 body for a failed struct validation:
// which field(s) failed, and why, rather than a generic "invalid request"
// string.
type validationErrorResponse struct {
	Error  string            `json:"error"`
	Fields map[string]string `json:"fields"`
}

// validateStruct runs req through the shared validator. On failure it
// writes a 400 response naming the failing field(s) and rule(s) and
// reports false so the caller can return early; on success it reports
// true and writes nothing.
func validateStruct(w http.ResponseWriter, req any) bool {
	err := validate.Struct(req)
	if err == nil {
		return true
	}

	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		// Not a validation failure (e.g. req isn't a struct) — this is a
		// handler bug, not a client error.
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return false
	}

	fields := make(map[string]string, len(verrs))
	for _, fe := range verrs {
		fields[fe.Field()] = fieldErrorMessage(fe)
	}

	writeJSON(w, http.StatusBadRequest, validationErrorResponse{
		Error:  "validation failed",
		Fields: fields,
	})
	return false
}

func fieldErrorMessage(fe validator.FieldError) string {
	if fe.Tag() == "required" {
		return fe.Field() + " is required"
	}
	return fe.Field() + " failed the '" + fe.Tag() + "' rule"
}
