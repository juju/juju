// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

var (
	// checkProvisionedStrategy defines the evil uninterruptible
	// retry strategy for "handling" ErrNotProvisioned. It exists
	// in the name of stability; as the code evolves, it would be
	// great to see its function moved up a level or two.
	//
	// TODO(katco): 2016-08-09: lp:1611427
	checkProvisionedStrategy = utils.AttemptStrategy{
		Total: 10 * time.Minute,
		Delay: 5 * time.Second,
	}

	// newConnFacade should similarly move up a level so it can
	// be explicitly configured without export_test hackery
	newConnFacade = apiagent.NewConnFacade

	// errAgentEntityDead is an internal error returned by getEntity.
	errAgentEntityDead = errors.New("agent entity is dead")

	// ErrConnectImpossible indicates that we can contact an apiserver
	// but have no hope of authenticating a connection with it.
	ErrConnectImpossible = errors.New("connection permanently impossible")

	// ErrChangedPassword indicates that the agent config used to connect
	// has been updated with a new password, and you should try again.
	ErrChangedPassword = errors.New("insecure password replaced; retry")
)

// OnlyConnect logs into the API using the supplied agent's credentials.
func OnlyConnect(a agent.Agent, apiOpen api.OpenFunc, logger Logger) (api.Connection, error) {
	agentConfig := a.CurrentConfig()
	info, ok := agentConfig.APIInfo()
	if !ok {
		return nil, errors.New("API info not available")
	}
	conn, _, err := connectFallback(apiOpen, info, agentConfig.OldPassword(), logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return conn, nil
}

// connectFallback opens an API connection using the supplied info,
// or a copy using the fallbackPassword; blocks for up to 5 minutes
// if it encounters a CodeNotProvisioned error, periodically retrying;
// and eventually, having either succeeded, failed, or timed out, returns:
//
//   * (if successful) the connection, and whether the fallback was used
//   * (otherwise) whatever error it most recently encountered
//
// It's clear that it still has machine-agent concerns still baked in,
// but there's no obvious practical path to separating those entirely at
// the moment.
//
// (The right answer is probably to treat CodeNotProvisioned as a normal
// error and depend on (currently nonexistent) exponential backoff in
// the framework: either it'll work soon enough, or the controller will
// spot the error and nuke the machine anyway. No harm leaving the local
// agent running and occasionally polling for changes -- it won't do much
// until it's managed to log in, and any suicide-cutoff point we pick here
// will be objectively bad in some circumstances.)
func connectFallback(
	apiOpen api.OpenFunc, info *api.Info, fallbackPassword string, logger Logger,
) (
	conn api.Connection, didFallback bool, err error,
) {
	// We expect to assign to `conn`, `err`, *and* `info` in
	// the course of this operation: wrapping this repeated
	// atom in a func currently seems to be less treacherous
	// than the alternatives.
	var tryConnect = func() {
		conn, err = apiOpen(info, api.DialOpts{
			// The DialTimeout is for connecting to the underlying
			// socket. We use three seconds because it should be fast
			// but it is possible to add a manual machine to a distant
			// controller such that the round trip time could be as high
			// as 500ms.
			DialTimeout: 3 * time.Second,
			// The delay between connecting to a different controller. Setting this to 0 means we try all controllers
			// simultaneously. We set it to approximately how long the TLS handshake takes, to avoid doing TLS
			// handshakes to a controller that we are going to end up ignoring.
			DialAddressInterval: 200 * time.Millisecond,
			// The timeout is for the complete login handshake.
			// If the server is rate limiting, it will normally pause
			// before responding to the login request, but the pause is
			// in the realm of five to ten seconds.
			Timeout: time.Minute,
		})
	}

	didFallback = info.Password == ""
	// Try to connect, trying both the primary and fallback
	// passwords if necessary; and update info, and remember
	// which password we used.
	if !didFallback {
		logger.Debugf("connecting with current password")
		tryConnect()
		if params.IsCodeUnauthorized(err) || errors.Cause(err) == common.ErrBadCreds {
			didFallback = true

		}
	}
	if didFallback {
		// We've perhaps used the wrong password, so
		// try again with the fallback password.
		infoCopy := *info
		info = &infoCopy
		info.Password = fallbackPassword
		logger.Debugf("connecting with old password")
		tryConnect()
	}

	// We might be a machine agent that's started before its
	// provisioner has had a chance to report instance data
	// to the machine; wait a fair while to ensure we really
	// are in the (expected rare) provisioner-crash situation
	// that would cause permanent CodeNotProvisioned (which
	// indicates that the controller has forgotten about us,
	// and is provisioning a new instance, so we really should
	// uninstall).
	//
	// Yes, it's dumb that this can't be interrupted, and that
	// it's not configurable without patching.
	if params.IsCodeNotProvisioned(err) {
		for a := checkProvisionedStrategy.Start(); a.Next(); {
			tryConnect()
			if !params.IsCodeNotProvisioned(err) {
				break
			}
		}
	}

	// At this point we've run out of reasons to retry connecting,
	// and just go with whatever error we last saw (if any).
	if err != nil {
		logger.Debugf("[%s] failed to connect", shortModelUUID(info.ModelTag))
		return nil, false, errors.Trace(err)
	}
	logger.Infof("[%s] %q successfully connected to %q",
		shortModelUUID(info.ModelTag),
		info.Tag.String(),
		conn.Addr())
	return conn, didFallback, nil
}

func shortModelUUID(model names.ModelTag) string {
	uuid := model.Id()
	if len(uuid) > 6 {
		return uuid[:6]
	}
	return uuid
}

// ScaryConnect logs into the API using the supplied agent's credentials,
// like OnlyConnect; and then:
//
//   * returns ErrConnectImpossible if the agent entity is dead or
//     unauthorized for all known passwords;
//   * replaces insecure credentials with freshly (locally) generated ones
//     (and returns ErrPasswordChanged, expecting to be reinvoked);
//   * unconditionally resets the remote-state password to its current value
//     (for what seems like a bad reason).
//
// This is clearly a mess but at least now it's a documented and localized
// mess; it should be used only when making the primary API connection for
// a machine or unit agent running in its own process.
func ScaryConnect(a agent.Agent, apiOpen api.OpenFunc, logger Logger) (_ api.Connection, err error) {
	agentConfig := a.CurrentConfig()
	info, ok := agentConfig.APIInfo()
	if !ok {
		return nil, errors.New("API info not available")
	}
	oldPassword := agentConfig.OldPassword()

	defer func() {
		cause := errors.Cause(err)
		switch {
		case cause == apiagent.ErrDenied:
		case cause == errAgentEntityDead:
		case params.IsCodeUnauthorized(cause):
		case params.IsCodeNotProvisioned(cause):
		default:
			return
		}
		logger.Errorf("Failed to connect to controller: %v", err)
		err = ErrConnectImpossible
	}()

	// Start connection...
	conn, usedOldPassword, err := connectFallback(apiOpen, info, oldPassword, logger)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// ...and make sure we close it if anything goes wrong.
	defer func() {
		if err != nil {
			if err := conn.Close(); err != nil {
				logger.Errorf("while closing API connection: %v", err)
			}
		}
	}()

	// newConnFacade is patched out in export_test, because exhaustion.
	// proper config/params struct would be better.
	facade, err := newConnFacade(conn)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// First of all, see if we're dead or removed, which will render
	// any further work pointless.
	entity := agentConfig.Tag()
	life, err := facade.Life(entity)
	if err != nil {
		return nil, errors.Trace(err)
	}
	switch life {
	case apiagent.Alive, apiagent.Dying:
	case apiagent.Dead:
		return nil, errAgentEntityDead
	default:
		return nil, errors.Errorf("unknown life value %q", life)
	}

	// If we need to change the password, it's far cleaner to
	// exit with ErrChangedPassword and depend on the framework
	// for expeditious retry than it is to mess around with those
	// responsibilities in here.
	if usedOldPassword {
		logger.Debugf("changing password...")
		err := changePassword(oldPassword, a, facade)
		if err != nil {
			return nil, errors.Trace(err)
		}
		logger.Infof("[%s] password changed for %q",
			shortModelUUID(agentConfig.Model()), entity.String())
		return nil, ErrChangedPassword
	}

	// If we *didn't* need to change the password, we apparently need
	// to reset our password to its current value anyway. Reportedly,
	// a machine agent promoted to controller status might have bad
	// auth data in mongodb, and this "fixes" it... but this is scary,
	// wrong, coincidental duct tape. The RTTD is to make controller-
	// promotion work correctly in the first place.
	//
	// Still, can't fix everything at once.
	if err := facade.SetPassword(entity, info.Password); err != nil {
		return nil, errors.Annotate(err, "can't reset agent password")
	}
	return conn, nil
}

// changePassword generates a new random password and records it in
// local agent configuration and on the remote state server. The supplied
// oldPassword -- which must be the current valid password -- is set as a
// fallback in local config, in case we fail to update the remote password.
func changePassword(oldPassword string, a agent.Agent, facade apiagent.ConnFacade) error {
	newPassword, err := utils.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	if err := a.ChangeConfig(func(c agent.ConfigSetter) error {
		c.SetPassword(newPassword)
		c.SetOldPassword(oldPassword)
		return nil
	}); err != nil {
		return errors.Trace(err)
	}
	// This has to happen *after* we record the old/new passwords
	// locally, lest we change it remotely, crash suddenly, and
	// end up locked out forever.
	return facade.SetPassword(a.CurrentConfig().Tag(), newPassword)
}

// NewExternalControllerConnectionFunc returns a function returning an
// api connection to a controller with the specified api info.
type NewExternalControllerConnectionFunc func(*api.Info) (api.Connection, error)

// NewExternalControllerConnection returns an api connection to a controller
// with the specified api info.
func NewExternalControllerConnection(apiInfo *api.Info) (api.Connection, error) {
	return api.Open(apiInfo, api.DialOpts{
		Timeout:    2 * time.Second,
		RetryDelay: 500 * time.Millisecond,
	})
}
