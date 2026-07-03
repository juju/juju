// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	coreerrors "github.com/juju/juju/core/errors"
	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/errors"
	pkissh "github.com/juju/juju/internal/pki/ssh"
	"github.com/juju/juju/internal/uuid"
)

// WatcherFactory describes watcher creation for SSH connection requests.
type WatcherFactory interface {
	NewNamespaceMapperWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		summary string,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption,
		filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)
}

// WatchableService extends Service with watcher support for SSH connection
// requests.
type WatchableService struct {
	*Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new model SSH service with watcher support.
func NewWatchableService(state State, modelUUID coremodel.UUID, clk clock.Clock, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service:        NewService(state, modelUUID, clk),
		watcherFactory: watcherFactory,
	}
}

// WatchSSHConnRequest returns a watcher that emits the tunnel IDs of SSH
// connection requests targeting the named machine as they are added in this
// model.
//
// This is used by the sshsession worker to watch for new connection requests
// destined for its own machine. The watcher is scoped to the machine so that a
// machine agent can only observe requests targeting itself. Only additions and
// updates are reported; deletions (consumed or expired requests) are not, as
// the worker only acts on newly added requests.
func (s *WatchableService) WatchSSHConnRequest(ctx context.Context, machineName coremachine.Name) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.state.PruneExpiredSSHConnRequests(ctx, s.clock.Now()); err != nil {
		return nil, errors.Errorf("pruning expired SSH connection requests: %w", err)
	}

	// Resolve the machine UUID up front so the watcher's initial query and
	// mapper are both scoped to this machine.
	machineUUID, err := s.state.GetMachineUUIDByName(ctx, machineName)
	if err != nil {
		return nil, errors.Errorf("getting UUID for machine %q: %w", machineName, err)
	}

	table, stmt := s.state.InitialWatchSSHConnRequestsStatement()

	// The changestream only carries the tunnel ID of a changed request, not the
	// machine it targets, so the mapper re-queries state to keep only the tunnel
	// IDs belonging to this machine.
	mapper := func(ctx context.Context, changes []changestream.ChangeEvent) ([]string, error) {
		if len(changes) == 0 {
			return nil, nil
		}
		tunnelIDs := make([]string, len(changes))
		for i, change := range changes {
			tunnelIDs[i] = change.Changed()
		}
		return s.state.FilterSSHConnRequestsForMachine(ctx, tunnelIDs, machineUUID)
	}

	w, err := s.watcherFactory.NewNamespaceMapperWatcher(
		ctx,
		eventsource.InitialNamespaceChanges(stmt, machineUUID),
		"ssh connection request watcher",
		mapper,
		eventsource.NamespaceFilter(table, changestream.Changed),
	)
	if err != nil {
		return nil, errors.Errorf("creating SSH connection request watcher: %w", err)
	}
	return w, nil
}

// Service provides model-scoped SSH operations: virtual host keys and
// one-shot SSH connection requests.
type Service struct {
	state     State
	modelUUID coremodel.UUID
	clock     clock.Clock
}

// NewService returns a new model SSH service.
func NewService(state State, modelUUID coremodel.UUID, clk clock.Clock) *Service {
	return &Service{
		state:     state,
		modelUUID: modelUUID,
		clock:     clk,
	}
}

// InsertSSHConnRequest stores a one-shot SSH connection request for this
// model.
//
// This is used by the tunneler worker to insert the connection request.
func (s *Service) InsertSSHConnRequest(ctx context.Context, req domainssh.SSHConnRequest) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if err := s.validateRequest(req); err != nil {
		return errors.Errorf("validating SSH connection request: %w", err)
	}

	if err := s.state.InsertSSHConnRequest(ctx, req, s.clock.Now()); err != nil {
		return errors.Errorf("inserting SSH connection request %q: %w", req.TunnelID, err)
	}
	return nil
}

// GetSSHConnRequest returns the SSH connection request for the supplied tunnel
// ID.
//
// This is used by the sshsession worker to retrieve a connection request.
func (s *Service) GetSSHConnRequest(ctx context.Context, tunnelID string) (domainssh.SSHConnRequest, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !uuid.IsValidUUIDString(tunnelID) {
		return domainssh.SSHConnRequest{}, errors.Errorf("tunnel id is not a uuid").Add(coreerrors.NotValid)
	}

	req, err := s.state.GetSSHConnRequest(ctx, tunnelID, s.clock.Now())
	if err != nil {
		return domainssh.SSHConnRequest{}, errors.Errorf("getting SSH connection request %q: %w", tunnelID, err)
	}
	return req, nil
}

// RemoveSSHConnRequest removes the SSH connection request for the supplied
// tunnel ID.
//
// This is used by the sshtunneler to remove a connection request after it has been used.
func (s *Service) RemoveSSHConnRequest(ctx context.Context, tunnelID string) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if !uuid.IsValidUUIDString(tunnelID) {
		return errors.Errorf("tunnel id is not a uuid").Add(coreerrors.NotValid)
	}

	if err := s.state.RemoveSSHConnRequest(ctx, tunnelID); err != nil {
		return errors.Errorf("removing SSH connection request %q: %w", tunnelID, err)
	}
	return nil
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
		machineName, ok := info.Machine()
		if !ok {
			return "", errors.Errorf("missing machine target in virtual hostname")
		}
		if machineName.IsContainer() {
			return "", errors.Errorf(
				"cannot SSH directly to nested machine %q, connect to parent machine %q instead",
				machineName,
				machineName.Parent(),
			)
		}
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
	// Fast path: check if the machine virtual host key already exists in state.
	key, found, err := s.state.GetMachineVirtualHostKeyByMachineName(ctx, machineName)
	if err != nil {
		return "", errors.Errorf("getting machine virtual SSH host key for %q: %w", machineName, err)
	}
	if found {
		return key, nil
	}

	// Slow path: generate a new machine virtual host key and persist it in state.
	// The state method handles cases of concurrent requests for the same machine.
	key, err = generateHostKey()
	if err != nil {
		return "", errors.Errorf("generating machine virtual SSH host key for %q: %w", machineName, err)
	}
	key, err = s.state.EnsureMachineVirtualHostKeyByMachineName(
		ctx,
		machineName,
		domainssh.SSHKeyAlgorithmTypeED25519ID,
		key,
	)
	if err != nil {
		return "", errors.Errorf("ensuring machine virtual SSH host key for %q: %w", machineName, err)
	}
	return key, nil
}

func (s *Service) ensureUnitVirtualHostKey(ctx context.Context, unitName string) (string, error) {
	// Fast path: check if the unit virtual host key already exists in state.
	key, found, err := s.state.GetUnitVirtualHostKeyByUnitName(ctx, unitName)
	if err != nil {
		return "", errors.Errorf("getting unit virtual SSH host key for %q: %w", unitName, err)
	}
	if found {
		return key, nil
	}

	// Slow path: generate a new unit virtual host key and persist it in state.
	// The state method handles cases of concurrent requests for the same unit.
	key, err = generateHostKey()
	if err != nil {
		return "", errors.Errorf("generating unit virtual SSH host key for %q: %w", unitName, err)
	}
	key, err = s.state.EnsureUnitVirtualHostKeyByUnitName(
		ctx,
		unitName,
		domainssh.SSHKeyAlgorithmTypeED25519ID,
		key,
	)
	if err != nil {
		return "", errors.Errorf("ensuring unit virtual SSH host key for %q: %w", unitName, err)
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

func (s *Service) validateRequest(req domainssh.SSHConnRequest) error {
	if !uuid.IsValidUUIDString(req.TunnelID) {
		return errors.Errorf("tunnel id is not a uuid").Add(coreerrors.NotValid)
	}
	if req.MachineName == "" {
		return errors.Errorf("empty machine name").Add(coreerrors.NotValid)
	}
	if err := coremachine.Name(req.MachineName).Validate(); err != nil {
		return errors.Errorf("validating machine name %q: %w", req.MachineName, err)
	}
	if req.SSHUsername == "" {
		return errors.Errorf("empty username").Add(coreerrors.NotValid)
	}
	if req.SSHPassword == "" {
		return errors.Errorf("empty password").Add(coreerrors.NotValid)
	}
	if req.Expires.IsZero() {
		return errors.Errorf("empty expiry").Add(coreerrors.NotValid)
	}
	if req.Expires.Before(s.clock.Now()) {
		return errors.Errorf("expiry %v is in the past", req.Expires.UTC()).Add(coreerrors.NotValid)
	}
	if len(req.ControllerAddresses) == 0 {
		return errors.Errorf("empty controller addresses").Add(coreerrors.NotValid)
	}
	if req.UnitPort <= 0 {
		return errors.Errorf("invalid unit port %d", req.UnitPort).Add(coreerrors.NotValid)
	}
	if len(req.EphemeralPublicKey) == 0 {
		return errors.Errorf("empty ephemeral public key").Add(coreerrors.NotValid)
	}

	return nil
}
