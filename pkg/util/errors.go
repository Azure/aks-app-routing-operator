package util

import (
	"errors"
	"fmt"
)

// NotFoundError is an error returned when a service returns a 404
type NotFoundError struct {
	Body string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("not found: %s", e.Body)
}

// IsNotFound returns true if the error is a NotFoundError
func IsNotFound(err error) bool {
	var notFoundErr *NotFoundError
	return errors.As(err, &notFoundErr)
}
