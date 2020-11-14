package cerberus

import (
	"errors"
	"fmt"
)

// ErrorCode cerberus unique error codes.
type ErrorCode int

const (
	_ ErrorCode = iota
	// ErrGeneric indicates a general error.
	ErrGeneric
	// ErrSaveServiceCfg indicates error while saving the service configuration.
	ErrSaveServiceCfg
	// ErrLoadServiceCfg indicates error while loading the service configuration.
	ErrLoadServiceCfg
	// ErrInstallService indicats error while installing a service.
	ErrInstallService
	// ErrUpdateService indicats error while updating a service.
	ErrUpdateService
	// ErrInvalidConfiguration indicats error while validating service configuration.
	ErrInvalidConfiguration
	// ErrRemoveService indicates error while removing a service.
	ErrRemoveService
	// ErrRunService indicates error while running a service.
	ErrRunService
	// ErrTimeout indicates an action run into a timeout.
	ErrTimeout
	// ErrSCMConnect indicates a failure while connecting to the SCM.
	ErrSCMConnect
)

var errorMap = map[ErrorCode]string{
	ErrSaveServiceCfg:       "SaveServiceCfg",
	ErrLoadServiceCfg:       "LoadServiceCfg",
	ErrInstallService:       "InstallService",
	ErrUpdateService:        "UpdateService",
	ErrRemoveService:        "RemoveService",
	ErrRunService:           "RunService",
	ErrInvalidConfiguration: "Validation",
}

// Error is a cerberus specific error.
type Error struct {
	Code      ErrorCode
	Message   string
	nestedErr error
}

// Is implements the errors.Is interface.
func (e Error) Is(target error) bool {
	var terr Error
	if errors.As(target, &terr) && terr.Code == e.Code {
		return true
	}

	return false
}

// Unwrap implements the errors.Unwrap interface.
func (e Error) Unwrap(err error) error {
	if e.nestedErr != nil {
		return e.nestedErr
	}

	return nil
}

// Error implements the error interface.
func (e Error) Error() string {
	err := e.Message
	if v, ok := errorMap[e.Code]; ok && v != "" {
		err = fmt.Sprintf("(%v) %v", v, e.Message)
	}

	if e.nestedErr != nil {
		err = fmt.Sprintf("%v: %v", err, e.nestedErr)
	}
	return err
}

// newError returns a new cerberus error.
func newError(code ErrorCode, message string, args ...interface{}) Error {
	return Error{Code: code, Message: fmt.Sprintf(message, args...)}
}

// newErrorW returns a new cerberus error and wraps an existing error.
func newErrorW(code ErrorCode, message string, err error, args ...interface{}) Error {
	e := newError(code, message)
	e.nestedErr = err
	return e
}
