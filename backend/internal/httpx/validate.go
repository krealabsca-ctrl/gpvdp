package httpx

import "github.com/go-playground/validator/v10"

// Validate es la instancia compartida del validador de structs de request.
var Validate = validator.New(validator.WithRequiredStructEnabled())
