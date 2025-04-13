package serrors

import (
	"errors"
)

// Error is a structured error that stores structured errors and values alongside the error
type Error struct {
	error
	keysAndValues []any
}

// Unwrap returns the unwrapped error
func (e *Error) Unwrap() error {
	return e.error
}

// Error returns the string representation of the error
func (e *Error) Error() string {
	return e.error.Error()
}

// WithValues injects additional structured keys and values into the error
func (e *Error) WithValues(keysAndValues ...any) *Error {
	e.keysAndValues = append(e.keysAndValues, keysAndValues...)
	return e
}

// Wrap wraps and existing error with additional structured keys and values
func Wrap(err error, keysAndValues ...any) error {
	if len(keysAndValues)%2 != 0 {
		panic("keysAndValues must have an even number of elements")
	}
	for i := 0; i < len(keysAndValues); i += 2 {
		if _, ok := keysAndValues[i].(string); !ok {
			panic("keys must be strings")
		}
	}
	return &Error{error: err, keysAndValues: keysAndValues}
}

// UnwrapValues returns a combined set of keys and values from every wrapped error
func UnwrapValues(err error) (values []any) {
	for err != nil {
		if v, ok := err.(*Error); ok {
			values = append(values, v.keysAndValues...)
		}
		err = errors.Unwrap(err)
	}
	if len(values) == 0 {
		return nil
	}
	return values
}
