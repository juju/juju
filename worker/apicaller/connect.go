// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	apiagent "github.com/juju/juju/api/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

var (
	checkProvisionedStrategy = utils.AttemptStrategy{
		Total: 1 * time.Minute,
		Delay: 5 * time.Second,
	}
	ErrAgentEntityDead = errors.New("agent entity is dead")
	ErrChangedPassword = errors.New("insecure password replaced; retry")
)

// APIOpen is an api.OpenFunc that wraps api.Open, and handles the edge
// case where a model has jumping several versions and doesn't yet have
// the model UUID cached in the agent config; in which case we fall back
// to login version 1.
//
// You probably want to use this in ManifoldConfig; *we* probably want to
// put this particular hack inside api.Open, but I seem to recall there
// being some complication last time I thought that was a good idea.
func APIOpen(info *api.Info, opts api.DialOpts) (api.Connection, error) {
	if info.ModelTag.Id() == "" {
		return api.OpenWithVersion(info, opts, 1)
	}
	return api.Open(info, opts)
}

// OnlyConnect logs into the API using the supplied agent's credentials.
func OnlyConnect(a agent.Agent, apiOpen api.OpenFunc) (api.Connection, error) {
	agentConfig := a.CurrentConfig()
	info, ok := agentConfig.APIInfo()
	if !ok {
		return nil, errors.New("API info not available")
	}
	conn, _, err := connectFallback(apiOpen, info, agentConfig.OldPassword())
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
	apiOpen api.OpenFunc, info *api.Info, fallbackPassword string,
) (
	conn api.Connection, didFallback bool, err error,
) {

	// We expect to assign to `conn`, `err`, *and* `info` in
	// the course of this operation: wrapping this repeated
	// atom in a func currently seems to be less treacherous
	// than the alternatives.
	var tryConnect = func() {
		conn, err = apiOpen(info, api.DialOpts{})
	}

	// Try to connect, trying both the primary and fallback
	// passwords if necessary; and update info, and remember
	// which password we used.
	tryConnect()
	if params.IsCodeUnauthorized(err) {
		// We've perhaps used the wrong password, so
		// try again with the fallback password.
		infoCopy := *info
		info = &infoCopy
		info.Password = fallbackPassword
		didFallback = true
		tryConnect()
	}

	// We might be a machine agent that's started before its
	// provisioner has had a chance to report instance data
	// to the machine; wait a fair while to ensure we really
	// are in the (expected rare) provisioner-crash situation
	// that would cause permanent CodeNotProvisioned.
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
		return nil, false, errors.Trace(err)
	}
	return conn, didFallback, nil
}

// ScaryConnect logs into the API using the supplied agent's credentials,
// like OnlyConnect; and then:
//
//   * returns ErrTerminateAgent if the agent entity is dead or unauthorized
//     for all known passwords;
//   * if the agent's config does not specify a model, tries to record
//     the model we just connected to;
//   * replaces insecure credentials with freshly (locally) generated ones
//     (and returns ErrPasswordChanged, expecting to be reinvoked);
//   * unconditionally resets the remote-state password to its current value
//     (for what seems like a bad reason).
//
// This is clearly a mess but at least now it's a documented and localized
// mess; it should be used in place of OnlyConnect when creating an API
// connection on behalf of an agent running in its own process (i.e. do
// NOT use it in a model agent).
func ScaryConnect(a agent.Agent, apiOpen api.OpenFunc) (_ api.Connection, err error) {
	agentConfig := a.CurrentConfig()
	info, ok := agentConfig.APIInfo()
	if !ok {
		return nil, errors.New("API info not available")
	}
	oldPassword := agentConfig.OldPassword()

	// Note: the ErrTerminateAgent returned when the connection is
	// known to be invalid or impossible will *not* terminate a
	// machine agent *unless* someone calls agent.SetCanUninstall.
	//
	// It's not sensible to do it in here, because calling that
	// func for a unit agent -- *or* for a model agent, which
	// looks like a machine agent on casual inspection -- will
	// cause the hosting machine agent to suicide if it gets a
	// particular signal, and that would be Very Bad.
	defer func() {
		cause := errors.Cause(err)
		switch {
		case cause == ErrAgentEntityDead:
		case params.IsCodeUnauthorized(cause):
		case params.IsCodeNotProvisioned(cause):
		default:
			return
		}
		return worker.ErrTerminateAgent
	}()

	// Start connection...
	conn, usedOldPassword, err := connectFallback(apiOpen, info, oldPassword)
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

	// Update the agent config if necessary; then get the entity we're
	// connecting as, or fail out if it's dead.
	maybeSetAgentModelTag(a, conn)
	entity, err := getEntity(conn, agentConfig.Tag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If we need to change the password, it's far cleaner to
	// exit with ErrChangedPassword and expect expeditious retry
	// than it is to mess around reassigning to conn and handling
	// the defer subtleties.
	if usedOldPassword {
		logger.Debugf("replacing insecure password")
		err := setAgentPassword(oldPassword, a, entity)
		if err != nil {
			return nil, errors.Trace(err)
		}
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
	if err := entity.SetPassword(info.Password); err != nil {
		return nil, errors.Annotate(err, "can't reset agent password")
	}
	return conn, nil
}

// maybeSetAgentModelTag tries to update the agent configuration if
// it's missing a model tag. It doesn't *really* matter if it fails,
// because we can demonstrably connect without it, so we log any
// errors encountered and never return any to the client.
func maybeSetAgentModelTag(a agent.Agent, conn api.Connection) {
	if a.CurrentConfig().Model().Id() == "" {
		err := a.ChangeConfig(func(setter agent.ConfigSetter) error {
			modelTag, err := conn.ModelTag()
			if err != nil {
				return errors.Annotate(err, "no model uuid set on api")
			}
			return setter.Migrate(agent.MigrateParams{
				Model: modelTag,
			})
		})
		if err != nil {
			logger.Warningf("unable to save model uuid: %v", err)
			// Not really fatal, just annoying.
		}
	}
}

// getEntity gets an entity, but returns ErrAgentEntityDead when appropriate
// for special handling by the client.
func getEntity(conn api.Connection, tag names.Tag) (*apiagent.Entity, error) {

	entity, err := conn.Agent().Entity(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	life := entity.Life()
	switch life {
	case params.Alive, params.Dying:
		return entity, nil
	case params.Dead:
		return nil, ErrAgentEntityDead
	}
	return nil, errors.Errorf("unknown entity life value: %v", life)
}

// setAgentPassword generates a new random password and records it in
// local agent configuration and on the remote state server. The supplied
// oldPassword is set as a fallback in local config in case the remote
// update fails and leaves the password unchanged.
func setAgentPassword(oldPassword string, a agent.Agent, entity *apiagent.Entity) error {
	newPassword, err := utils.RandomPassword()
	if err != nil {
		return errors.Trace(err)
	}
	if err := a.ChangeConfig(func(c agent.ConfigSetter) error {
		c.SetPassword(newPassword)
		c.SetOldPassword(oldPassword)
		return nil
	}); err != nil {
		return err
	}
	// This has to happen *after* we record the old/new passwords
	// locally, lest we change it remotely, crash suddenly, and
	// end up locked out forever.
	return entity.SetPassword(newPassword)
}
