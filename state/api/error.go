package api
import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/rpc"
)

// Error is the type of error returned by any call
// to the state API.
type Error struct {
	Message string
	Code string
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) ErrorCode() string {
	return e.Code
}

var singletonErrorCodes = map[error] string {
	state.ErrUnauthorized: CodeUnauthorized,
	state.ErrCannotEnterScopeYet: CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope: CodeCannotEnterScope,
	state.ErrExcessiveContention: CodeExcessiveContention,
	state.ErrUnitHasSubordinates: CodeUnitHasSubordinates,
}

// The Code constants hold error codes for some kinds of error.
const (
	CodeNotFound = "not found"
	CodeUnauthorized = "unauthorized access"
	CodeCannotEnterScope = "cannot enter scope"
	CodeCannotEnterScopeYet = "cannot enter scope yet"
	CodeExcessiveContention = "excessive contention"
	CodeUnitHasSubordinates = "unit has subordinates"
	CodeNotAssigned = "not assigned"
)

func serverError(err error) error {
	code := singletonErrorCodes[err]
	switch {
	case code != "":
	case state.IsNotFound(err):
		code = CodeNotFound
	case state.IsNotAssigned(err):
		code = CodeNotAssigned
	}
	if code != "" {
		return &rpc.ServerError{
			Message: err.Error(),
			Code: code,
		}
	}
	return err
}

// ErrCode returns the error code associated with
// the given error, or the empty string if there
// is none.
func ErrCode(err error) string {
	if err := err.(rpc.ErrorCoder); err != nil {
		return err.ErrorCode()
	}
	return ""
}

// clientError maps errors returned from an RPC call into local errors with
// appropriate values.
func clientError(err error) error {
	rerr, ok := err.(*rpc.ServerError)
	if !ok {
		return err
	}
	// We use our own error type rather than rpc.ServerError
	// because we don't want the code or the "server error" prefix
	// within the error message. Also, it's best not to make clients
	// know that we're using the rpc package.
	return &Error{
		Message: rerr.Message,
		Code: rerr.Code,
	}
}
