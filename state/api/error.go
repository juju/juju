package api
import (
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/log"
	"errors"
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

var _ rpc.ErrorCoder = (*Error)(nil)

var (
	errBadId       = errors.New("id not found")
	errBadCreds    = errors.New("invalid entity name or password")
	errPerm        = errors.New("permission denied")
	errNotLoggedIn = errors.New("not logged in")
)

var singletonErrorCodes = map[error] string {
	state.ErrUnauthorized: CodeUnauthorized,
	state.ErrCannotEnterScopeYet: CodeCannotEnterScopeYet,
	state.ErrCannotEnterScope: CodeCannotEnterScope,
	state.ErrExcessiveContention: CodeExcessiveContention,
	state.ErrUnitHasSubordinates: CodeUnitHasSubordinates,
	errBadId: CodeNotFound,
	errBadCreds: CodeUnauthorized,
	errPerm: CodeUnauthorized,
	errNotLoggedIn: CodeUnauthorized,
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
	err1 := serverError1(err)
	log.Printf("error %#v -> %#v", err, err1)
	return err1
}

func serverError1(err error) error {
	code := singletonErrorCodes[err]
	switch {
	case code != "":
	case state.IsNotFound(err):
		code = CodeNotFound
	case state.IsNotAssigned(err):
		code = CodeNotAssigned
	}
	if code != "" {
		return &Error{
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
	log.Printf("clientError %#v", err)
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
