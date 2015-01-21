// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// isMachineWithJob returns whether the given entity is a machine that
// is configured to run the given job.
func isMachineWithJob(e state.Entity, j state.MachineJob) bool {
	m, ok := e.(*state.Machine)
	if !ok {
		return false
	}
	for _, mj := range m.Jobs() {
		if mj == j {
			return true
		}
	}
	return false
}

func setPassword(e state.Authenticator, password string) error {
	// Catch expected common case of misspelled
	// or missing Password parameter.
	if password == "" {
		return errors.New("password is empty")
	}
	return e.SetPassword(password)
}

type validateArgs struct {
	st      *state.State
	envUUID string
	// strict validation does not allow empty UUID values
	strict bool
	// stateServerEnvOnly only validates the state server environment
	stateServerEnvOnly bool
}

// validateEnvironUUID is the common validator for the various apiserver
// components that need to check for a valid environment UUID.
// An empty envUUID means that the connection has come in at the root
// of the URL space and refers to the state server environment.
// The *state.State parameter is expected to be the state server State
// connection.  The return *state.State is a connection for the specified
// environment UUID if the UUID refers to an environment contained in the
// database.  If the bool return value is true, the state connection should
// be closed at the end of serving the client connection.
func validateEnvironUUID(args validateArgs) (*state.State, bool, error) {
	if args.envUUID == "" {
		// We allow the environUUID to be empty for 2 cases
		// 1) Compatibility with older clients
		// 2) TODO: server a limited API at the root (empty envUUID)
		//    with just the user manager and environment manager
		//    if the connection comes over a sufficiently up to date
		//    login command.
		if args.strict {
			return nil, false, errors.Trace(common.UnknownEnvironmentError(args.envUUID))
		}
		logger.Debugf("validate env uuid: empty envUUID")
		return args.st, false, nil
	}
	if args.envUUID == args.st.EnvironUUID() {
		logger.Debugf("validate env uuid: state server environment - %s", args.envUUID)
		return args.st, false, nil
	}
	if args.stateServerEnvOnly || !names.IsValidEnvironment(args.envUUID) {
		return nil, false, errors.Trace(common.UnknownEnvironmentError(args.envUUID))
	}
	envTag := names.NewEnvironTag(args.envUUID)
	if _, err := args.st.GetEnvironment(envTag); err != nil {
		return nil, false, errors.Wrap(err, common.UnknownEnvironmentError(args.envUUID))
	}
	logger.Debugf("validate env uuid: %s", args.envUUID)
	result, err := args.st.ForEnviron(envTag)
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	return result, true, nil
}
