package common

// Caller interface is implemented by the client-facing State object.
type Caller interface {
	Call(objType, id, request string, params, response interface{}) error
}
