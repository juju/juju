// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/state"
)

// UpgradeSeries defines an interface for interacting with upgrading a series.
type UpgradeSeries interface {

	// Validate validates that the incoming arguments correspond to a
	// valid series upgrade for the target machine.
	// If they do, a list of the machine's current units is returned for use in
	// soliciting user confirmation of the command.
	Validate(context.Context, []ValidationEntity) ([]ValidationResult, error)

	// Prepare attempts to prepare a machine for a OS series upgrade.
	// It is expected that a validate call has been performed before the prepare
	// step.
	Prepare(context.Context, string, string, bool) error

	// Complete will complete the upgrade series.
	Complete(string) error
}

// UpgradeSeriesState defines a common set of functions for retrieving state
// objects.
type UpgradeSeriesState interface {
	// MachineFromTag returns a machine from a given tag.
	// Returns an error if the machine is not found.
	MachineFromTag(string) (Machine, error)

	// ApplicationsFromMachine returns a list of all the applications for a
	// given machine. This includes all the subordinates.
	ApplicationsFromMachine(Machine) ([]Application, error)
}

// ApplicationValidator defines an application validator. It aims to just
// validate a series of applications for a set series.
type ApplicationValidator interface {
	// ValidateApplications attempts to validate a series of applications for
	// a given series. Using force to allow the overriding of the error to
	// ensure all applications validate.
	//
	// I do question if you actually need to validate anything if force is
	// employed here?
	ValidateApplications(ctx context.Context, applications []Application, base corebase.Base, force bool) error
}

// UpgradeBaseValidator defines a set of validators for the upgrade series
// scenarios.
type UpgradeBaseValidator interface {
	ApplicationValidator

	// ValidateBase validates a given requested base against the current
	// machine base.
	// The machine tag is currently used for descriptive information and could
	// be deprecated in reality.
	ValidateBase(requestedBase, machineBase corebase.Base, machineTag string) error

	// ValidateMachine validates a given machine for ensuring it meets a given
	// state (quiescence essentially) and has no current ongoing machine lock.
	ValidateMachine(Machine) error

	// ValidateUnits validates a given set of units.
	ValidateUnits([]Unit) error
}

// UpgradeSeriesAPI provides the upgrade series API facade for any given
// version. It is expected that any API parameter changes should be performed
// before entering the API.
type UpgradeSeriesAPI struct {
	state      UpgradeSeriesState
	validator  UpgradeBaseValidator
	authorizer Authorizer
}

// NewUpgradeSeriesAPI creates a new UpgradeSeriesAPI
func NewUpgradeSeriesAPI(
	state UpgradeSeriesState,
	validator UpgradeBaseValidator,
	authorizer Authorizer,
) *UpgradeSeriesAPI {
	return &UpgradeSeriesAPI{
		state:      state,
		validator:  validator,
		authorizer: authorizer,
	}
}

// ValidationEntity defines a type that requires validation.
type ValidationEntity struct {
	Tag     string
	Channel string
	Force   bool
}

// ValidationResult defines the result of the validation.
type ValidationResult struct {
	Error     error
	UnitNames []string
}

// Validate validates that the incoming arguments correspond to a
// valid series upgrade for the target machine.
// If they do, a list of the machine's current units is returned for use in
// soliciting user confirmation of the command.
func (a *UpgradeSeriesAPI) Validate(ctx context.Context, entities []ValidationEntity) ([]ValidationResult, error) {
	if err := a.authorizer.CanRead(); err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]ValidationResult, len(entities))
	for i, entity := range entities {
		machine, err := a.state.MachineFromTag(entity.Tag)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		if err := a.validator.ValidateMachine(machine); err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		requestedBase, err := baseFromParams(entity.Tag, machine.Base(), entity.Channel)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}
		if err := a.validateApplication(ctx, machine, requestedBase, entity.Force); err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		units, err := machine.Units()
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		if err := a.validator.ValidateUnits(units); err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		unitNames := make([]string, len(units))
		for i, unit := range units {
			unitNames[i] = unit.UnitTag().Id()
		}

		results[i].UnitNames = unitNames
	}

	return results, nil
}

func (a *UpgradeSeriesAPI) Prepare(ctx context.Context, tag, channel string, force bool) (retErr error) {
	machine, err := a.state.MachineFromTag(tag)
	if err != nil {
		return errors.Trace(err)
	}

	requestedBase, err := baseFromParams(tag, machine.Base(), channel)
	if err != nil {
		return errors.Trace(err)
	}

	if err := a.validator.ValidateMachine(machine); err != nil {
		return errors.Trace(err)
	}

	units, err := machine.Units()
	if err != nil {
		return errors.Trace(err)
	}
	unitNames := make([]string, len(units))
	for i, unit := range units {
		unitNames[i] = unit.UnitTag().Id()
	}

	// Validate the machine applications for a given series.
	if err := a.validateApplication(ctx, machine, requestedBase, force); err != nil {
		return errors.Trace(err)
	}

	// TODO 2018-06-28 managed series upgrade
	// improve error handling based on error type, there will be cases where retrying
	// the hooks is needed etc.
	err = machine.CreateUpgradeSeriesLock(unitNames, state.Base{OS: requestedBase.OS, Channel: requestedBase.Channel.Track})
	if err != nil {
		return errors.Trace(err)
	}

	// We're inside a series lock transaction. It is required that we remove
	// the series lock upon any error.
	defer func() {
		if retErr == nil {
			return
		}
		if err := machine.RemoveUpgradeSeriesLock(); err != nil {
			retErr = errors.Annotatef(retErr, "%s occurred while cleaning up from", err)
		}
	}()

	// Once validated, set the machine status to started.
	mBase, err := corebase.ParseBase(machine.Base().OS, machine.Base().Channel)
	if err != nil {
		return errors.Trace(err)
	}
	message := fmt.Sprintf("started upgrade from %q to %q", mBase.DisplayString(), requestedBase.DisplayString())
	return machine.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareStarted, message)
}

func (a *UpgradeSeriesAPI) Complete(tag string) error {
	machine, err := a.state.MachineFromTag(tag)
	if err != nil {
		return errors.Trace(err)
	}
	return machine.CompleteUpgradeSeries()
}

func (a *UpgradeSeriesAPI) validateApplication(ctx context.Context, machine Machine, requestedBase corebase.Base, force bool) error {
	base, err := corebase.ParseBase(machine.Base().OS, machine.Base().Channel)
	if err != nil {
		return errors.Trace(err)
	}
	if err := a.validator.ValidateBase(requestedBase, base, machine.Tag().String()); err != nil {
		return errors.Trace(err)
	}

	// The following returns all the applications including subordinates for a
	// given machine. Validating all applications that are from different stores
	// is also supported.
	applications, err := a.state.ApplicationsFromMachine(machine)
	if err != nil {
		return errors.Trace(err)
	}

	if err := a.validator.ValidateApplications(ctx, applications, requestedBase, force); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type upgradeSeriesState struct {
	state Backend
}

func (s upgradeSeriesState) MachineFromTag(tag string) (Machine, error) {
	machineTag, err := names.ParseMachineTag(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	machine, err := s.state.Machine(machineTag.Id())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return machine, nil
}

func (s upgradeSeriesState) ApplicationsFromMachine(machine Machine) ([]Application, error) {
	names, err := machine.ApplicationNames()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Ensure that we have unique names for this application request.
	names = set.NewStrings(names...).Values()

	results := make([]Application, len(names))
	for i, name := range names {
		app, err := s.state.Application(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		results[i] = app
	}
	return results, nil
}

type upgradeSeriesValidator struct {
	localValidator  ApplicationValidator
	remoteValidator ApplicationValidator
}

func makeUpgradeSeriesValidator(client CharmhubClient) upgradeSeriesValidator {
	return upgradeSeriesValidator{
		localValidator: stateSeriesValidator{},
		remoteValidator: charmhubSeriesValidator{
			client: client,
		},
	}
}

func baseFromParams(machineTag string, base state.Base, channel string) (corebase.Base, error) {
	if base.OS != "ubuntu" {
		return corebase.Base{}, errors.Errorf("%s is running %s and is not valid for Ubuntu series upgrade",
			machineTag, base.OS)
	}
	if channel == "" {
		return corebase.Base{}, errors.New("channel missing from args")
	}
	return corebase.ParseBase(base.OS, channel)
}

// ValidateBase validates a given requested base against the current
// machine base.
func (s upgradeSeriesValidator) ValidateBase(requestedBase, machineBase corebase.Base, machineTag string) error {
	if requestedBase.String() == "" {
		return errors.BadRequestf("base missing from args")
	}

	if requestedBase.OS != "ubuntu" {
		return errors.Errorf("base %q is not a valid upgrade target", requestedBase)
	}

	if requestedBase.Channel.Track == machineBase.Channel.Track {
		return errors.Errorf("%s is already running base %s", machineTag, requestedBase.DisplayString())
	}

	// TODO (Check the charmhub API for all applications running on this) machine
	// to see if it's possible to run a charm on this machine.

	isOlderBase, err := isBaseLessThan(requestedBase, machineBase)
	if err != nil {
		return errors.Trace(err)
	}
	if isOlderBase {
		return errors.Errorf("machine %s is running %s which is a newer base than %s.",
			machineTag, machineBase.DisplayString(), requestedBase.DisplayString())
	}

	return nil
}

// ValidateApplications attempts to validate a series of applications for
// a given series.
func (s upgradeSeriesValidator) ValidateApplications(ctx context.Context, applications []Application, base corebase.Base, force bool) error {
	// We do it this way, so we can batch the charmhub charm queries. This is
	// leaking an implementation detail into the decision logic, but we can't
	// work around that.
	var (
		stateApps   []Application
		requestApps []Application
	)
	for _, app := range applications {
		origin := app.CharmOrigin()

		// This is not a charmhub charm, so we can fallback to querying state
		// for the supported series.
		if origin == nil || !corecharm.CharmHub.Matches(origin.Source) {
			stateApps = append(stateApps, app)
			continue
		}

		requestApps = append(requestApps, app)
	}

	if err := s.localValidator.ValidateApplications(ctx, stateApps, base, force); err != nil {
		return errors.Trace(err)
	}

	return s.remoteValidator.ValidateApplications(ctx, requestApps, base, force)
}

// ValidateMachine validates a given machine for ensuring it meets a given
// state (quiescence essentially) and has no current ongoing machine lock.
func (s upgradeSeriesValidator) ValidateMachine(machine Machine) error {
	if machine.IsManager() {
		return errors.Errorf("%s is a controller and cannot be targeted for series upgrade", machine.Tag().String())
	}

	// If we've already got a series lock on upgrade, don't go any further.
	locked, err := machine.IsLockedForSeriesUpgrade()
	if err != nil {
		return errors.Trace(err)
	}
	if !locked {
		return nil
	}
	// Grab the status from upgrade series and add it to the error.
	status, err := machine.UpgradeSeriesStatus()
	if err != nil {
		return errors.Trace(err)
	}

	// Additionally add the status to the underlying params error. This
	// gives a typed error to the client, which can then decode ths
	// optional information later on.
	return &apiservererrors.UpgradeSeriesValidationError{
		Cause:  errors.Errorf("upgrade series lock found for %q; series upgrade is in the %q state", machine.Id(), status),
		Status: status.String(),
	}
}

// ValidateUnits validates a given set of units.
func (s upgradeSeriesValidator) ValidateUnits(units []Unit) error {
	for _, u := range units {
		agentStatus, err := u.AgentStatus()
		if err != nil {
			return errors.Trace(err)
		}
		if agentStatus.Status != status.Idle {
			return errors.Errorf("unit %s is not ready to start a series upgrade; its agent status is: %q %s",
				u.Name(), agentStatus.Status, agentStatus.Message)
		}
		unitStatus, err := u.Status()
		if err != nil {
			return errors.Trace(err)
		}
		if unitStatus.Status == status.Error {
			return errors.Errorf("unit %s is not ready to start a series upgrade; its status is: \"error\" %s",
				u.Name(), unitStatus.Message)
		}
	}
	return nil
}

// stateSeriesValidator validates an application using the state (database)
// version of the charm.
// NOTE: stateSeriesValidator also exists in apiserver/facades/client/application/update_series.go,
// When making changes here, review the copy for required changes as well.
type stateSeriesValidator struct{}

// ValidateApplications attempts to validate a series of applications for
// a given base.
func (s stateSeriesValidator) ValidateApplications(ctx context.Context, applications []Application, base corebase.Base, force bool) error {
	if len(applications) == 0 {
		return nil
	}

	for _, app := range applications {
		if err := s.verifySupportedBase(app, base, force); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (s stateSeriesValidator) verifySupportedBase(application Application, base corebase.Base, force bool) error {
	ch, _, err := application.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	supportedBases, err := corecharm.ComputedBases(ch)
	if err != nil {
		return errors.Trace(err)
	}
	if len(supportedBases) == 0 {
		err := errors.NewNotSupported(nil, fmt.Sprintf("charm %q does not support any bases. Not valid", ch.Meta().Name))
		return apiservererrors.ServerError(err)
	}
	_, baseSupportedErr := corecharm.BaseForCharm(base, supportedBases)
	if baseSupportedErr != nil && !force {
		return apiservererrors.NewErrIncompatibleBase(supportedBases, base, ch.Meta().Name)
	}
	return nil
}

// NOTE: charmhubSeriesValidator also exists in apiserver/facades/client/application/update_series.go,
// When making changes here, review the copy for required changes as well.
type charmhubSeriesValidator struct {
	client CharmhubClient
}

// ValidateApplications attempts to validate a series of applications for
// a given base.
func (s charmhubSeriesValidator) ValidateApplications(ctx context.Context, applications []Application, base corebase.Base, force bool) error {
	if len(applications) == 0 {
		return nil
	}

	configs := make([]charmhub.RefreshConfig, len(applications))
	for i, app := range applications {
		// We can be assured that the charm origin is not nil, because we
		// guarded against it before.
		origin := app.CharmOrigin()
		rev := origin.Revision
		if rev == nil {
			return errors.Errorf("no revision found for application %q", app.Name())
		}

		cfg, err := charmhub.DownloadOneFromRevision(origin.ID, *rev)
		if err != nil {
			return errors.Trace(err)
		}
		configs[i] = cfg
	}
	refreshResp, err := s.client.Refresh(ctx, charmhub.RefreshMany(configs...))
	if err != nil {
		return errors.Trace(err)
	}
	if len(refreshResp) != len(applications) {
		return errors.Errorf("unexpected number of responses %d for applications %d", len(refreshResp), len(applications))
	}
	for _, resp := range refreshResp {
		if err := resp.Error; err != nil && !force {
			return errors.Annotatef(err, "unable to locate application with base %s", base.DisplayString())
		}
	}
	// DownloadOneFromRevision does not take a base, however the response contains the bases
	// supported by the given revision.  Validate against provided series.
	channelToValidate := base.Channel.Track
	if err != nil {
		return errors.Trace(err)
	}
	for _, resp := range refreshResp {
		var found bool
		for _, base := range resp.Entity.Bases {
			channel, err := corebase.ParseChannel(base.Channel)
			if err != nil {
				return errors.Trace(err)
			}
			if channelToValidate == channel.Track || force {
				found = true
				break
			}
		}
		if !found {
			return errors.Errorf("charm %q does not support %s, force not used", resp.Name, base)
		}
	}
	return nil
}
