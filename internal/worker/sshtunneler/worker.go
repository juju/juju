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

// TunnelTracker is the interface exposed by the worker for creating,
// authenticating and pushing reverse SSH tunnels. It is implemented by
// [sshtunneler.Tracker].
type TunnelTracker interface {
	// RequestTunnel requests a reverse SSH tunnel to a model-specific machine
	// and blocks until the tunnel is established or the context is cancelled.
	RequestTunnel(ctx context.Context, req sshtunneler.RequestArgs) (*gossh.Client, error)
	// AuthenticateTunnel authenticates an incoming reverse-tunnel SSH request
	// and returns the tunnel ID to pass to PushTunnel.
	AuthenticateTunnel(username, password string) (tunnelID string, err error)
	// PushTunnel publishes an established network connection for the tunnel
	// identified by tunnelID.
	PushTunnel(ctx context.Context, tunnelID string, conn net.Conn) error
}

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
	// GetSSHHostKeysByMachineName returns the SSH host keys stored for the
	// machine identified by its name.
	GetSSHHostKeysByMachineName(ctx context.Context, name coremachine.Name) ([]string, error)
}

// ControllerNodeService provides controller-scoped address lookups used to
// build the reverse-tunnel callback candidate list.
type ControllerNodeService interface {
	// GetAPIAddressesForControllerIDForAgents returns the agent-reachable API
	// addresses for the given controller node ID, ordered to prefer
	// cloud-local addresses.
	GetAPIAddressesForControllerIDForAgents(ctx context.Context, controllerID string) ([]string, error)
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

	keys, err := machineSvc.GetSSHHostKeysByMachineName(ctx, coremachine.Name(machineID))
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

// LocalAddresses implements sshtunneler.ControllerInfo.
// It returns the API addresses for the given controller node as
// SpaceAddresses. Only the local node's addresses are returned so that
// the machine's reverse tunnel connects back to the node holding the
// waiting client SSH session.
func (a *controllerInfoAdapter) LocalAddresses(ctx context.Context, controllerNodeID string) (network.SpaceAddresses, error) {
	addrs, err := a.controllerNodeService.GetAPIAddressesForControllerIDForAgents(ctx, controllerNodeID)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get controller node addresses")
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
		return tomb.ErrDying
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
