// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"strconv"

	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/virtualhostname"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/errors"
	pkissh "github.com/juju/juju/internal/pki/ssh"
)

// Service provides model-scoped SSH virtual host key workflows.
type Service struct {
	modelUUID coremodel.UUID
	state     State
}

// NewService returns a new model SSH service.
func NewService(modelUUID coremodel.UUID, state State) *Service {
	return &Service{
		modelUUID: modelUUID,
		state:     state,
	}
}

// VirtualHostKey resolves the terminating SSH host key for a virtual hostname.
func (s *Service) VirtualHostKey(ctx context.Context, info virtualhostname.Info) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	modelUUID := info.ModelUUID()
	if err := modelUUID.Validate(); err != nil {
		return "", errors.Errorf("validating model UUID %q: %w", modelUUID, err)
	}
	if modelUUID != s.modelUUID {
		// This is a programmatic error that should never occur, as the service should have been
		// created with the correct model UUID beforehand. We return an error here to be defensive.
		return "", errors.Errorf("virtual hostname model UUID %q does not match service model %q", modelUUID, s.modelUUID)
	}

	switch info.Target() {
	case virtualhostname.MachineTarget:
		machineNumber, ok := info.Machine()
		if !ok {
			return "", errors.Errorf("missing machine target in virtual hostname")
		}
		machineName := coremachine.Name(strconv.Itoa(machineNumber))
		return s.MachineVirtualHostKey(ctx, machineName)
	case virtualhostname.UnitTarget, virtualhostname.ContainerTarget:
		unitName, ok := info.Unit()
		if !ok {
			return "", errors.Errorf("missing unit target in virtual hostname")
		}
		parsedUnitName, err := coreunit.NewName(unitName)
		if err != nil {
			return "", errors.Errorf("validating unit name %q: %w", unitName, err)
		}
		return s.UnitVirtualHostKey(ctx, parsedUnitName)
	default:
		return "", errors.Errorf("virtual hostname target %d %w", info.Target(), coreerrors.NotSupported)
	}
}

// MachineVirtualHostKey returns the machine terminating host key, generating
// and persisting it if it is missing.
func (s *Service) MachineVirtualHostKey(ctx context.Context, machineName coremachine.Name) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := machineName.Validate(); err != nil {
		return "", errors.Errorf("validating machine name %q: %w", machineName, err)
	}

	return s.ensureMachineVirtualHostKey(ctx, machineName.String())
}

// UnitVirtualHostKey returns the terminating host key for a unit target.
// IAAS units share the host key of their backing machine, while CAAS units use
// a unit-scoped virtual host key.
func (s *Service) UnitVirtualHostKey(ctx context.Context, unitName coreunit.Name) (string, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := unitName.Validate(); err != nil {
		return "", errors.Errorf("validating unit name %q: %w", unitName, err)
	}

	machineName, machineBacked, err := s.state.GetMachineNameForUnit(ctx, unitName.String())
	if err != nil {
		return "", errors.Errorf("resolving backing machine for unit %q in model %q: %w", unitName, s.modelUUID, err)
	}
	if machineBacked {
		return s.ensureMachineVirtualHostKey(ctx, machineName)
	}
	return s.ensureUnitVirtualHostKey(ctx, unitName.String())
}

func (s *Service) ensureMachineVirtualHostKey(ctx context.Context, machineName string) (string, error) {
	key, found, err := s.state.GetMachineVirtualHostKeyByMachineName(ctx, machineName)
	if err != nil {
		return "", errors.Errorf("getting machine virtual SSH host key for %q: %w", machineName, err)
	}
	if found {
		return key, nil
	}

	key, err = generateHostKey()
	if err != nil {
		return "", errors.Errorf("generating machine virtual SSH host key for %q: %w", machineName, err)
	}
	if err := s.state.SetMachineVirtualHostKeyByMachineName(ctx, machineName, domainssh.SSHKeyAlgorithmTypeED25519ID, key); err != nil {
		return "", errors.Errorf("persisting machine virtual SSH host key for %q: %w", machineName, err)
	}
	return key, nil
}

func (s *Service) ensureUnitVirtualHostKey(ctx context.Context, unitName string) (string, error) {
	key, found, err := s.state.GetUnitVirtualHostKeyByUnitName(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting unit virtual SSH host key for %q: %w", unitName, err)
	}
	if found {
		return key, nil
	}

	key, err = generateHostKey()
	if err != nil {
		return "", errors.Errorf("generating unit virtual SSH host key for %q: %w", unitName, err)
	}
	if err := s.state.SetUnitVirtualHostKeyByUnitName(ctx, unitName, domainssh.SSHKeyAlgorithmTypeED25519ID, key); err != nil {
		return "", errors.Errorf("persisting unit virtual SSH host key for %q: %w", unitName, err)
	}
	return key, nil
}

func generateHostKey() (string, error) {
	key, err := pkissh.NewMarshalledED25519()
	if err != nil {
		return "", errors.Capture(err)
	}
	return string(key), nil
}
