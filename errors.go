package jsonql

import "fmt"

// JsonQLError is the base interface for all JSONQL errors.
type JsonQLError interface {
	error
	Code() string
}

// ValidationError is already defined in types.go (the individual error item).
// JsonQLValidationError wraps a set of validation errors into a single error.
type JsonQLValidationError struct {
	Errors []ValidationError
}

func (e *JsonQLValidationError) Error() string {
	if len(e.Errors) == 0 {
		return "validation failed"
	}
	return fmt.Sprintf("validation failed: %s", e.Errors[0].Message)
}

func (e *JsonQLValidationError) Code() string { return "VALIDATION_ERROR" }

// JsonQLTranspileError is returned when SQL generation fails.
type JsonQLTranspileError struct {
	Msg string
}

func (e *JsonQLTranspileError) Error() string { return e.Msg }
func (e *JsonQLTranspileError) Code() string  { return "TRANSPILE_ERROR" }

// JsonQLExecutionError is returned when database execution fails.
type JsonQLExecutionError struct {
	Msg   string
	Cause error
}

func (e *JsonQLExecutionError) Error() string { return e.Msg }
func (e *JsonQLExecutionError) Code() string  { return "EXECUTION_ERROR" }
func (e *JsonQLExecutionError) Unwrap() error { return e.Cause }

// JsonQLParseError is returned when query parsing fails.
type JsonQLParseError struct {
	Msg   string
	Cause error
}

func (e *JsonQLParseError) Error() string { return e.Msg }
func (e *JsonQLParseError) Code() string  { return "PARSE_ERROR" }
func (e *JsonQLParseError) Unwrap() error { return e.Cause }
