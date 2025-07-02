// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/application"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/agentpassword"
	passworderrors "github.com/juju/juju/domain/agentpassword/errors"
	"github.com/juju/juju/internal/errors"
	internalpassword "github.com/juju/juju/internal/password"
)

// ModelState gets and sets the state of the service.
type ModelState interface {
	// GetUnitUUID returns the UUID of the unit with the given name.
	GetUnitUUID(context.Context, unit.Name) (unit.UUID, error)

	// SetUnitPasswordHash sets the password hash for the given unit.
	SetUnitPasswordHash(context.Context, unit.UUID, agentpassword.PasswordHash) error

	// MatchesUnitPasswordHash checks if the password is valid or not against
	// the password hash stored in the database.
	MatchesUnitPasswordHash(context.Context, unit.UUID, agentpassword.PasswordHash) (bool, error)

	// GetMachineUUID returns the UUID of the machine with the given name.
	GetMachineUUID(context.Context, machine.Name) (machine.UUID, error)

	// SetMachinePasswordHash sets the password hash for the given machine.
	SetMachinePasswordHash(context.Context, machine.UUID, agentpassword.PasswordHash) error

	// MatchesMachinePasswordHashWithNonce checks if the password is valid or
	// not against the password hash with the nonce stored in the database.
	MatchesMachinePasswordHashWithNonce(context.Context, machine.UUID, agentpassword.PasswordHash, string) (bool, error)

	// IsMachineController returns whether the machine is a controller machine.
	// It returns a NotFound if the given machine doesn't exist.
	IsMachineController(context.Context, machine.Name) (bool, error)

	// SetApplicationPasswordHash sets the password hash for the given application.
	SetApplicationPasswordHash(context.Context, application.ID, agentpassword.PasswordHash) error

	// MatchesApplicationPasswordHash checks if the password is valid or not against the
	// password hash stored in the database.
	MatchesApplicationPasswordHash(context.Context, application.ID, agentpassword.PasswordHash) (bool, error)

	// GetApplicationIDByName returns the application ID for the named application.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationIDByName(ctx context.Context, name string) (application.ID, error)
}

// ControllerState gets and sets the state of the controller node service.
type ControllerState interface {
	// SetControllerNodePasswordHash sets the password hash for the given
	// controller node.
	SetControllerNodePasswordHash(context.Context, string, agentpassword.PasswordHash) error
	// MatchesControllerNodePasswordHash checks if the password is valid or not
	// against the password hash stored in the database for the controller node.
	MatchesControllerNodePasswordHash(context.Context, string, agentpassword.PasswordHash) (bool, error)
}

// Service provides the means for interacting with the passwords in a model.
type Service struct {
	modelState      ModelState
	controllerState ControllerState
}

// NewService returns a new Service.
func NewService(modelState ModelState, controllerState ControllerState) *Service {
	return &Service{
		modelState:      modelState,
		controllerState: controllerState,
	}
}

// SetUnitPassword sets the password for the given unit. If the unit does not
// exist, an error satisfying [passworderrors.UnitNotFound] is returned.
func (s *Service) SetUnitPassword(ctx context.Context, unitName unit.Name, password string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if len(password) < internalpassword.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	unitUUID, err := s.modelState.GetUnitUUID(ctx, unitName)
	if err != nil {
		return errors.Capture(err)
	}

	return s.modelState.SetUnitPasswordHash(ctx, unitUUID, hashPassword(password))
}

// MatchesUnitPasswordHash checks if the password is valid or not against the
// password hash stored in the database.
func (s *Service) MatchesUnitPasswordHash(ctx context.Context, unitName unit.Name, password string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return false, errors.Capture(err)
	}

	// An empty password is never valid.
	if password == "" {
		return false, passworderrors.EmptyPassword
	} else if len(password) < internalpassword.MinAgentPasswordLength {
		return false, errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	unitUUID, err := s.modelState.GetUnitUUID(ctx, unitName)
	if err != nil {
		return false, errors.Capture(err)
	}

	return s.modelState.MatchesUnitPasswordHash(ctx, unitUUID, hashPassword(password))
}

// SetMachinePassword sets the password for the given machine. If the machine
// does not exist, an error satisfying [passworderrors.MachineNotFound] is
// returned.
func (s *Service) SetMachinePassword(ctx context.Context, machineName machine.Name, password string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return errors.Capture(err)
	}
	if len(password) < internalpassword.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	machineUUID, err := s.modelState.GetMachineUUID(ctx, machineName)
	if err != nil {
		return errors.Capture(err)
	}

	return s.modelState.SetMachinePasswordHash(ctx, machineUUID, hashPassword(password))
}

// MatchesMachinePasswordHashWithNonce checks if the password with a nonce is
// valid or not against the password hash stored in the database.
func (s *Service) MatchesMachinePasswordHashWithNonce(ctx context.Context, machineName machine.Name, password, nonce string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return false, errors.Capture(err)
	}

	// An empty password is never valid.
	if password == "" {
		return false, passworderrors.EmptyPassword
	} else if len(password) < internalpassword.MinAgentPasswordLength {
		return false, errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	if nonce == "" {
		return false, passworderrors.EmptyNonce
	}

	machineUUID, err := s.modelState.GetMachineUUID(ctx, machineName)
	if err != nil {
		return false, errors.Capture(err)
	}

	return s.modelState.MatchesMachinePasswordHashWithNonce(ctx, machineUUID, hashPassword(password), nonce)
}

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (s *Service) IsMachineController(ctx context.Context, machineName machine.Name) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	isController, err := s.modelState.IsMachineController(ctx, machineName)
	if err != nil {
		return false, errors.Errorf("checking if machine %q is a controller: %w", machineName, err)
	}
	return isController, nil
}

// SetControllerNodePassword sets the password for the given controller node.
func (s *Service) SetControllerNodePassword(ctx context.Context, id string, password string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if id == "" {
		return errors.Errorf("controller node ID %w", coreerrors.NotValid)
	}
	if len(password) < internalpassword.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	return s.controllerState.SetControllerNodePasswordHash(ctx, id, hashPassword(password))
}

// MatchesControllerNodePasswordHash checks if the password is
// valid or not against the password hash stored in the database.
func (s *Service) MatchesControllerNodePasswordHash(ctx context.Context, id, password string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if id == "" {
		return false, errors.Errorf("controller node ID %w", coreerrors.NotValid)
	}

	// An empty password is never valid.
	if password == "" {
		return false, passworderrors.EmptyPassword
	} else if len(password) < internalpassword.MinAgentPasswordLength {
		return false, errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	return s.controllerState.MatchesControllerNodePasswordHash(ctx, id, hashPassword(password))
}

// SetApplicationPassword sets the password for the given application. If the
// app does not exist, an error satisfying [applicationerrors.ApplicationNotFound]
// is returned.
func (s *Service) SetApplicationPassword(ctx context.Context, appID application.ID, password string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(password) < internalpassword.MinAgentPasswordLength {
		return errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	return s.modelState.SetApplicationPasswordHash(ctx, appID, hashPassword(password))
}

// MatchesApplicationPasswordHash checks if the password is valid or not against
// the password hash stored in the database.
func (s *Service) MatchesApplicationPasswordHash(ctx context.Context, appName string, password string) (bool, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// An empty password is never valid.
	if password == "" {
		return false, passworderrors.EmptyPassword
	} else if len(password) < internalpassword.MinAgentPasswordLength {
		return false, errors.Errorf("password is only %d bytes long, and is not a valid Agent password: %w", len(password), passworderrors.InvalidPassword)
	}

	appID, err := s.modelState.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return false, errors.Capture(err)
	}

	return s.modelState.MatchesApplicationPasswordHash(ctx, appID, hashPassword(password))
}

func hashPassword(p string) agentpassword.PasswordHash {
	return agentpassword.PasswordHash(internalpassword.AgentPasswordHash(p))
}
