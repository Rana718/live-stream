package middleware

import (
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	validatorOnce sync.Once
	validate      *validator.Validate
)

// Validator returns a process-wide validator instance.
func Validator() *validator.Validate {
	validatorOnce.Do(func() {
		validate = validator.New(validator.WithRequiredStructEnabled())
	})
	return validate
}

// ValidateStruct runs validation and returns a flat error message.
func ValidateStruct(s any) error {
	return Validator().Struct(s)
}
