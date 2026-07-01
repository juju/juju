// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"net"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	gossh "golang.org/x/crypto/ssh"
	"gopkg.in/tomb.v2"

	coremachine "github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/sshtunneler"
)

// SSHModelService provides model-scoped SSH connection-request operations
// needed by the tunnel tracker.
type SSHModelService interface {
	// InsertSSHConnRequest stores a one-shot reverse tunnel request for this
	// model.
	InsertSSHConnRequest(ctx context.Context, req domainssh.SSHConnRequest) error
}

// MachineService provides model-scoped machine SSH host key lookups needed by
// the tunnel tracker to verify the identity of the machine when the reverse
// tunnel is established.
type MachineService interface {
	// GetMachineUUID returns the UUID of a machine identified by its name.
	GetMachineUUID(ctx context.Context, name coremachine.Name) (coremachine.UUID, error)
	// GetSSHHostKeys returns the SSH host keys stored for the given machine.
	GetSSHHostKeys(ctx context.Context, mUUID coremachine.UUID) ([]string, error)
}

// ControllerNodeService provides controller-scoped address lookups used to
// build the reverse-tunnel callback candidate list.
type ControllerNodeService interface {
	// GetAllAPIAddressesForAgents returns the API addresses reachable by
	// agents, ordered to prefer cloud-local addresses.
	GetAllAPIAddressesForAgents(ctx context.Context) ([]string, error)
}

// GetSSHServiceFunc resolves a model-scoped SSHModelService from the domain
// services getter for a specific model UUID.
type GetSSHServiceFunc = func(ctx context.Context, getter services.DomainServicesGetter, modelUUID coremodel.UUID) (SSHModelService, error)

// GetMachineServiceFunc resolves a model-scoped MachineService from the domain
// services getter for a specific model UUID.
type GetMachineServiceFunc = func(ctx context.Context, getter services.DomainServicesGetter, modelUUID coremodel.UUID) (MachineService, error)

// sshTunnelerWorker is a worker that wraps a tunnel tracker singleton.
type sshTunnelerWorker struct {
	tomb          tomb.Tomb
	tunnelTracker *sshtunneler.Tracker
}

type sshDialer struct{}

// Dial establishes an SSH connection over the provided connection.
// It uses the provided username, private key, and host key callback for
// authentication.
func (d sshDialer) Dial(conn net.Conn, username string, privateKey gossh.Signer, hostKeyCallback gossh.HostKeyCallback) (*gossh.Client, error) {
	sshConn, newChan, reqs, err := gossh.NewClientConn(conn, "", &gossh.ClientConfig{
		HostKeyCallback: hostKeyCallback,
		User:            username,
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(privateKey),
		},
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to establish SSH connection")
	}
	return gossh.NewClient(sshConn, newChan, reqs), nil
}

// connRequestStateAdapter implements sshtunneler.ConnRequestState.
// It routes InsertSSHConnRequest to the model-scoped SSH service identified
// by the model UUID embedded in the request.
type connRequestStateAdapter struct {
	domainServicesGetter services.DomainServicesGetter
	getSSHService        GetSSHServiceFunc
}

// InsertSSHConnRequest implements sshtunneler.ConnRequestState.
func (a *connRequestStateAdapter) InsertSSHConnRequest(ctx context.Context, modelUUID coremodel.UUID, req domainssh.SSHConnRequest) error {
	svc, err := a.getSSHService(ctx, a.domainServicesGetter, modelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	return svc.InsertSSHConnRequest(ctx, req)
}

// machineStateAdapter implements sshtunneler.MachineState.
// It routes MachineHostKeys to the model-scoped machine service identified
// by the model UUID argument, resolving machine name to UUID before fetching
// registered SSH host keys.
type machineStateAdapter struct {
	domainServicesGetter services.DomainServicesGetter
	getMachineService    GetMachineServiceFunc
}

// MachineHostKeys implements sshtunneler.MachineState.
func (a *machineStateAdapter) MachineHostKeys(ctx context.Context, modelUUID, machineID string) ([]string, error) {
	parsedUUID := coremodel.UUID(modelUUID)
	if err := parsedUUID.Validate(); err != nil {
		return nil, errors.Annotatef(err, "invalid model UUID %q", modelUUID)
	}

	machineSvc, err := a.getMachineService(ctx, a.domainServicesGetter, parsedUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineUUID, err := machineSvc.GetMachineUUID(ctx, coremachine.Name(machineID))
	if err != nil {
		return nil, errors.Annotatef(err, "resolving UUID for machine %q", machineID)
	}

	keys, err := machineSvc.GetSSHHostKeys(ctx, machineUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "getting SSH host keys for machine %q", machineID)
	}
	return keys, nil
}

// controllerInfoAdapter adapts a ControllerNodeService to the
// sshtunneler.ControllerInfo interface, building SpaceAddresses from
// the address strings returned by the controller node service.
type controllerInfoAdapter struct {
	controllerNodeService ControllerNodeService
}

// Addresses implements sshtunneler.ControllerInfo.
// It returns all API addresses available for agents as SpaceAddresses.
func (a *controllerInfoAdapter) Addresses(ctx context.Context) (network.SpaceAddresses, error) {
	addrs, err := a.controllerNodeService.GetAllAPIAddressesForAgents(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get controller addresses")
	}
	return network.NewSpaceAddresses(addrs...), nil
}

// NewWorker returns a worker that provides an SSH tunnel tracker.
func NewWorker(domainServicesGetter services.DomainServicesGetter, getSSHService GetSSHServiceFunc, getMachineService GetMachineServiceFunc, controllerNodeService ControllerNodeService, clk clock.Clock) (worker.Worker, error) {
	args := sshtunneler.TrackerArgs{
		ConnRequestState: &connRequestStateAdapter{
			domainServicesGetter: domainServicesGetter,
			getSSHService:        getSSHService,
		},
		MachineState: &machineStateAdapter{
			domainServicesGetter: domainServicesGetter,
			getMachineService:    getMachineService,
		},
		ControllerInfo: &controllerInfoAdapter{
			controllerNodeService: controllerNodeService,
		},
		Dialer: sshDialer{},
		Clock:  clk,
	}
	tracker, err := sshtunneler.NewTracker(args)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create tunnel tracker")
	}

	w := &sshTunnelerWorker{tunnelTracker: tracker}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w, nil
}

// Kill is part of the worker.Worker interface.
func (w *sshTunnelerWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *sshTunnelerWorker) Wait() error {
	return w.tomb.Wait()
}
