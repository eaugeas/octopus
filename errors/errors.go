package errors

import "github.com/eaugeas/octopus/logs"

// Error is the response returned by the server when it fails
// to satisfy a request
type Error struct {
	// ErrorCode is a unique identifier for the error that can be used to identify
	// the particular type of error encountered
	ErrorCode int `json:"errorCode"`

	// Description is a human-readable description of the error that occurred
	// to aid the client in debugging
	Description string `json:"description"`
}

// Error is the implementation of go's error interface for Error
func (e *Error) Error() string {
	return e.Description
}

func (e *Error) Log(fields logs.Fields) {
	fields.Add("error_code", e.ErrorCode)
	fields.Add("description", e.Description)
}
