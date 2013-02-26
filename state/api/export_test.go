package api

import "launchpad.net/juju-core/rpc"

var (
	ServerError    = serverError
	ErrBadId       = errBadId
	ErrBadCreds    = errBadCreds
	ErrPerm        = errPerm
	ErrNotLoggedIn = errNotLoggedIn
)

// RPCClient returns the RPC client for the state, so that testing
// functions can tickle parts of the API that the conventional entry
// points don't reach.
func RPCClient(st *State) *rpc.Client {
	return st.client
}
