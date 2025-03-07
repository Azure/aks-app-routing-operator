package controllererrors

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserErrors(t *testing.T) {
	testMsg := "test error message"
	testError := NewUserError(errors.New("test"), testMsg)
	var userErr UserError

	assert.True(t, testError.UserError() == testMsg)
	assert.True(t, errors.As(testError, &userErr))
}
