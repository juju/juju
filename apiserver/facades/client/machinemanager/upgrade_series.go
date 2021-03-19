// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"fmt"

	"github.com/juju/charm/v8"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/charmhub"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/os"
	"github.com/juju/juju/core/series"
	"github.com/juju/juju/core/status"
	stateerrors "github.com/juju/juju/state/errors"
)

// UpgradeSeries defines an interface for interacting with upgrading a series.
type UpgradeSeries interface {

	// Validate validates that the incoming arguments correspond to a
	// valid series upgrade for the target machine.
	// If they do, a list of the machine's current units is returned for use in
	// soliciting user confirmation of the command.
	Validate([]ValidationEntity) ([]ValidationResult, error)

	// Prepare attempts to prepare a machine for a OS series upgrade.
	// It is expected that a validate call has been performed before the prepare
	// step.
	Prepare(string, string, bool) error
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
	ValidateApplications(applications []Application, series string, force bool) error
}

// UpgradeSeriesValidator defines a set of validators for the upgrade series
// scenarios.
type UpgradeSeriesValidator interface {
	ApplicationValidator

	// ValidateSeries validates a given requested series against the current
	// machine series.
	// The machine tag is currently used for descriptive information and could
	// be deprecated in reality.
	ValidateSeries(requestedSeries, machineSeries, machineTag string) error

	// ValidateMachine validates a given machine for ensuring it meets a given
	// state (quiescence essentially) and has no current ongoing machine lock.
	ValidateMachine(Machine) error
}

// UpgradeSeriesAPI provides the upgrade series API facade for any given
// version. It is expected that any API parameter changes should be performed
// before entering the API.
type UpgradeSeriesAPI struct {
	state      UpgradeSeriesState
	validator  UpgradeSeriesValidator
	authorizer Authorizer
}

// NewUpgradeSeriesAPI creates a new UpgradeSeriesAPI
func NewUpgradeSeriesAPI(
	state UpgradeSeriesState,
	validator UpgradeSeriesValidator,
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
	Tag    string
	Series string
	Force  bool
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
func (a *UpgradeSeriesAPI) Validate(entities []ValidationEntity) ([]ValidationResult, error) {
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

		if err := a.validateApplication(machine, entity.Series, entity.Force); err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		units, err := machine.Units()
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		unitNames, err := verifyUnits(units)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		results[i].UnitNames = unitNames
	}

	return results, nil
}

func (a *UpgradeSeriesAPI) Prepare(tag, series string, force bool) (retErr error) {
	if series == "" {
		return errors.BadRequestf("series missing from args")
	}

	machine, err := a.state.MachineFromTag(tag)
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

	// TODO 2018-06-28 managed series upgrade
	// improve error handling based on error type, there will be cases where retrying
	// the hooks is needed etc.
	err = machine.CreateUpgradeSeriesLock(unitNames, series)
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

	// Validate the machine applications for a given series.
	if err := a.validateApplication(machine, series, force); err != nil {
		return errors.Trace(err)
	}

	// Once validated, set the machine status to started.
	message := fmt.Sprintf("started upgrade series from %q to %q", machine.Series(), series)
	return machine.SetUpgradeSeriesStatus(model.UpgradeSeriesPrepareStarted, message)
}

func (a *UpgradeSeriesAPI) validateApplication(machine Machine, requestedSeries string, force bool) error {
	if err := a.validator.ValidateSeries(requestedSeries, machine.Series(), machine.Tag().String()); err != nil {
		return errors.Trace(err)
	}

	// The following returns all the applications including subordinates for a
	// given machine. Validating all applications that are from different stores
	// is also supported.
	applications, err := a.state.ApplicationsFromMachine(machine)
	if err != nil {
		return errors.Trace(err)
	}

	if err := a.validator.ValidateApplications(applications, requestedSeries, force); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func verifyUnits(units []Unit) ([]string, error) {
	unitNames := make([]string, len(units))
	for i, u := range units {
		agentStatus, err := u.AgentStatus()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if agentStatus.Status != status.Idle {
			return nil, errors.Errorf("unit %s is not ready to start a series upgrade; its agent status is: %q %s",
				u.Name(), agentStatus.Status, agentStatus.Message)
		}
		unitStatus, err := u.Status()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if unitStatus.Status == status.Error {
			return nil, errors.Errorf("unit %s is not ready to start a series upgrade; its status is: \"error\" %s",
				u.Name(), unitStatus.Message)
		}

		unitNames[i] = u.UnitTag().Id()
	}
	return unitNames, nil
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
	removeValidator ApplicationValidator
}

func makeUpgradeSeriesValidator(client CharmhubClient) upgradeSeriesValidator {
	return upgradeSeriesValidator{
		localValidator: stateSeriesValidator{},
		removeValidator: charmhubSeriesValidator{
			client: client,
		},
	}
}

// ValidateSeries validates a given requested series against the current
// machine series.
func (s upgradeSeriesValidator) ValidateSeries(requestedSeries, machineSeries, machineTag string) error {
	if requestedSeries == "" {
		return errors.BadRequestf("series missing from args")
	}

	opSys, err := series.GetOSFromSeries(requestedSeries)
	if err != nil {
		return errors.Trace(err)
	}
	if opSys != os.Ubuntu {
		return errors.Errorf("series %q is from OS %q and is not a valid upgrade target",
			requestedSeries, opSys.String())
	}

	opSys, err = series.GetOSFromSeries(machineSeries)
	if err != nil {
		return errors.Trace(err)
	}
	if opSys != os.Ubuntu {
		return errors.Errorf("%s is running %s and is not valid for Ubuntu series upgrade",
			machineTag, opSys.String())
	}

	if requestedSeries == machineSeries {
		return errors.Errorf("%s is already running series %s", machineTag, requestedSeries)
	}

	// TODO (Check the charmhub API for all applications running on this) machine
	// to see if it's possible to run a charm on this machine.

	isOlderSeries, err := isSeriesLessThan(requestedSeries, machineSeries)
	if err != nil {
		return errors.Trace(err)
	}
	if isOlderSeries {
		return errors.Errorf("machine %s is running %s which is a newer series than %s.",
			machineTag, machineSeries, requestedSeries)
	}

	return nil
}

// ValidateApplications attempts to validate a series of applications for
// a given series.
func (s upgradeSeriesValidator) ValidateApplications(applications []Application, series string, force bool) error {
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

	if err := s.localValidator.ValidateApplications(stateApps, series, force); err != nil {
		return errors.Trace(err)
	}

	return s.removeValidator.ValidateApplications(requestApps, series, force)
}

// ValidateMachine validates a given machine for ensuring it meets a given
// state (quiescence essentially) and has no current ongoing machine lock.
func (s upgradeSeriesValidator) ValidateMachine(machine Machine) error {
	if machine.IsManager() {
		return errors.Errorf("%s is a controller and cannot be targeted for series upgrade", machine.Tag().String())
	}

	// If we've already got a series lock on upgrade, don't go any further.
	if locked, err := machine.IsLockedForSeriesUpgrade(); errors.IsNotFound(errors.Cause(err)) {
		return errors.Trace(err)
	} else if locked {
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
	return nil
}

// stateSeriesValidator validates an application using the state (database)
// version of the charm.
type stateSeriesValidator struct{}

// ValidateApplications attempts to validate a series of applications for
// a given series.
func (s stateSeriesValidator) ValidateApplications(applications []Application, series string, force bool) error {
	if len(applications) == 0 {
		return nil
	}

	for _, app := range applications {
		if err := s.verifySupportedSeries(app, series, force); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (s stateSeriesValidator) verifySupportedSeries(application Application, series string, force bool) error {
	ch, _, err := application.Charm()
	if err != nil {
		return errors.Trace(err)
	}
	supportedSeries := ch.Meta().ComputedSeries()
	if len(supportedSeries) == 0 {
		supportedSeries = append(supportedSeries, ch.URL().Series)
	}
	_, seriesSupportedErr := charm.SeriesForCharm(series, supportedSeries)
	if seriesSupportedErr != nil && !force {
		// TODO (stickupkid): Once all commands are placed in this API, we
		// should relocate these to the API server.
		return stateerrors.NewErrIncompatibleSeries(supportedSeries, series, ch.String())
	}
	return nil
}

type charmhubSeriesValidator struct {
	client CharmhubClient
}

// ValidateApplications attempts to validate a series of applications for
// a given series.
func (s charmhubSeriesValidator) ValidateApplications(applications []Application, series string, force bool) error {
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

		platform := charmhub.RefreshPlatform{
			Architecture: origin.Platform.Architecture,
			OS:           origin.Platform.OS,
			Series:       series,
		}
		cfg, err := charmhub.DownloadOneFromRevision(origin.ID, *rev, platform)
		if err != nil {
			return errors.Trace(err)
		}
		configs[i] = cfg
	}
	refreshResp, err := s.client.Refresh(context.TODO(), charmhub.RefreshMany(configs...))
	if err != nil {
		return errors.Trace(err)
	}
	if len(refreshResp) != len(applications) {
		return errors.Errorf("unexpected number of responses %d for applications %d", len(refreshResp), len(applications))
	}
	for _, resp := range refreshResp {
		if err := resp.Error; err != nil && !force {
			return errors.Annotatef(err, "unable to locate application with series %s", series)
		}
	}
	return nil
}
