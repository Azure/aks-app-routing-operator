package util

type UserError struct {
	Err         error
	UserMessage string
}

// for internal use
func (b UserError) Error() string {
	return b.Err.Error()
}

// for user facing messages
func (b UserError) UserError() string {
	return b.UserMessage
}

func NewUserError(err error, msg string) UserError {
	return UserError{err, msg}
}
