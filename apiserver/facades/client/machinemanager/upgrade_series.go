// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/status"
)

// UpgradeSeries defines an interface for interacting with upgrading a series.
type UpgradeSeries interface {

	// Validate validates that the incoming arguments correspond to a
	// valid series upgrade for the target machine.
	// If they do, a list of the machine's current units is returned for use in
	// soliciting user confirmation of the command.
	Validate([]ValidationEntity) ([]ValidationResult, error)
}

// UpgradeSeriesAuthorizer checks to see if an upgrade series can be performed.
type UpgradeSeriesAuthorizer interface {
	// Read checks to see if a read is possible. Returns an error if a read is
	// not possible.
	Read() error

	// Write checks to see if a write is possible. Returns an error if a write
	// is not possible.
	Write() error
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

	// UnitsFromMachine returns a list of all the units for a given machine.
	UnitsFromMachine(Machine) ([]Unit, error)
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
	authorizer UpgradeSeriesAuthorizer
}

// NewUpgradeSeriesAPI creates a new UpgradeSeriesAPI
func NewUpgradeSeriesAPI(
	state UpgradeSeriesState,
	validator UpgradeSeriesValidator,
	authorizer UpgradeSeriesAuthorizer,
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
	if err := a.authorizer.Read(); err != nil {
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

		units, err := a.state.UnitsFromMachine(machine)
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

type upgradeSeriesAuthorizer struct {
	facade *MachineManagerAPI
}

// Read checks to see if a read is possible. Returns an error if a read is
// not possible.
func (a upgradeSeriesAuthorizer) Read() error {
	return a.facade.checkCanRead()
}

// Write checks to see if a write is possible. Returns an error if a write
// is not possible.
func (a upgradeSeriesAuthorizer) Write() error {
	return a.facade.checkCanWrite()
}

type upgradeSeriesState struct {
	facade *MachineManagerAPI
}

func (s upgradeSeriesState) MachineFromTag(tag string) (Machine, error) {
	return s.facade.machineFromTag(tag)
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
		app, err := s.facade.st.Application(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		results[i] = app
	}
	return results, nil
}

func (s upgradeSeriesState) UnitsFromMachine(machine Machine) ([]Unit, error) {
	return machine.Units()
}

type upgradeSeriesValidator struct {
	facade *MachineManagerAPI
}

func (s upgradeSeriesValidator) ValidateSeries(requested, machine, tag string) error {
	return s.facade.validateSeries(requested, machine, tag)
}

func (s upgradeSeriesValidator) ValidateApplications(applications []Application, series string, force bool) error {
	for _, app := range applications {
		if err := app.VerifySupportedSeries(series, force); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
