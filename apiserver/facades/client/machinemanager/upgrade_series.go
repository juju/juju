// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/charm/v9"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
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

// UpgradeSeriesValidator defines a set of validators for the upgrade series
// scenarios.
type UpgradeSeriesValidator interface {
	ValidateSeries(requestedSeries, machineSeries, machineTag string) error
	ValidateApplications(applications []Application, series string, force bool) error
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

		if machine.IsManager() {
			results[i].Error = errors.Errorf("%s is a controller and cannot be targeted for series upgrade", entity.Tag)
			continue
		}

		// If we've already got a series lock on upgrade, don't go any further.
		if locked, err := machine.IsLockedForSeriesUpgrade(); errors.IsNotFound(errors.Cause(err)) {
			results[i].Error = errors.Trace(err)
			continue
		} else if locked {
			// Grab the status from upgrade series and add it to the error.
			status, err := machine.UpgradeSeriesStatus()
			if err != nil {
				results[i].Error = errors.Trace(err)
				continue
			}

			// Additionally add the status to the underlying params error. This
			// gives a typed error to the client, which can then decode ths
			// optional information later on.
			results[i].Error = &apiservererrors.UpgradeSeriesValidationError{
				Cause:  errors.Errorf("upgrade series lock found for %q; series upgrade is in the %q state", machine.Id(), status),
				Status: status.String(),
			}
			continue
		}

		if err := a.validator.ValidateSeries(entity.Series, machine.Series(), entity.Tag); err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		applications, err := a.state.ApplicationsFromMachine(machine)
		if err != nil {
			results[i].Error = errors.Trace(err)
			continue
		}

		if err := a.validator.ValidateApplications(applications, entity.Series, entity.Force); err != nil {
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

type upgradeSeriesValidator struct{}

func (s upgradeSeriesValidator) ValidateSeries(requested, machine, tag string) error {
	return validateSeries(requested, machine, tag)
}

func (s upgradeSeriesValidator) ValidateApplications(applications []Application, series string, force bool) error {
	for _, app := range applications {
		if err := s.verifySupportedSeries(app, series, force); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (s upgradeSeriesValidator) verifySupportedSeries(application Application, series string, force bool) error {
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
