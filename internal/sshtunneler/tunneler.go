// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/pki/ssh"
	"github.com/juju/juju/internal/uuid"
)

var (
	maxTimeout = 60 * time.Second
)

const (
	reverseTunnelUser = "juju-reverse-tunnel"
	tokenIssuer       = "sshtunneler"
	tokenSubject      = "reverse-tunnel"
	tunnelIDClaimKey  = "tunnelID"
	defaultUser       = "ubuntu"
)

// ConnRequestState defines an interface to write SSH connection requests to
// model-scoped state.
type ConnRequestState interface {
	// InsertSSHConnRequest inserts a one-shot reverse tunnel request into the
	// model-scoped SSH connection request state identified by the model UUID.
	InsertSSHConnRequest(ctx context.Context, modelUUID model.UUID, args domainssh.SSHConnRequest) error
}

// MachineState defines an interface to read machine SSH host keys from
// model-scoped state.
type MachineState interface {
	// MachineHostKeys returns the SSH host keys registered for the given
	// machine in the specified model.
	MachineHostKeys(ctx context.Context, modelUUID, machineID string) ([]string, error)
}

// ControllerInfo defines an interface to fetch the controller's address.
type ControllerInfo interface {
	// Addresses returns the public API addresses of all controller nodes,
	// for use as reverse tunnel callback candidates.
	Addresses(ctx context.Context) (network.SpaceAddresses, error)
}

// SSHDialer defines an interface to establish an SSH connection over a provided connection.
type SSHDial interface {
	Dial(conn net.Conn, username string, privateKey gossh.Signer, hostKeyCallback gossh.HostKeyCallback) (*gossh.Client, error)
}

// Tracker provides methods to create SSH tunnels to machine units.
// The objects keep track of consumers who have requested tunnels
// and allows an SSH server to push tunnels to these consumers.
type Tracker struct {
	authn        tunnelAuthentication
	connRequests ConnRequestState
	machines     MachineState
	controller   ControllerInfo
	dialer       SSHDial
	clock        clock.Clock

	mu      sync.Mutex
	tracker map[string]chan (net.Conn)
}

// TrackerArgs holds the arguments for creating a new tunnel tracker.
type TrackerArgs struct {
	ConnRequestState ConnRequestState
	MachineState     MachineState
	ControllerInfo   ControllerInfo
	Dialer           SSHDial
	Clock            clock.Clock
}

func (args *TrackerArgs) validate() error {
	if args.ConnRequestState == nil {
		return errors.New("conn request state is required")
	}
	if args.MachineState == nil {
		return errors.New("machine state is required")
	}
	if args.ControllerInfo == nil {
		return errors.New("controller info is required")
	}
	if args.Dialer == nil {
		return errors.New("dialer is required")
	}
	if args.Clock == nil {
		return errors.New("clock is required")
	}
	return nil
}

// NewTracker creates a new tunnel tracker.
func NewTracker(args TrackerArgs) (*Tracker, error) {
	if err := args.validate(); err != nil {
		return nil, err
	}

	authn, err := newTunnelAuthentication(args.Clock)
	if err != nil {
		return nil, err
	}

	return &Tracker{
		tracker:      make(map[string]chan (net.Conn)),
		authn:        authn,
		controller:   args.ControllerInfo,
		clock:        args.Clock,
		connRequests: args.ConnRequestState,
		machines:     args.MachineState,
		dialer:       args.Dialer,
	}, nil
}

// RequestArgs holds the arguments for requesting a tunnel.
type RequestArgs struct {
	MachineID string
	ModelUUID string
}

func (tt *Tracker) generateEphemeralSSHKey() (gossh.Signer, gossh.PublicKey, error) {
	privKey, err := ssh.ED25519()
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to generate key")
	}

	sshPrivateKey, err := gossh.NewSignerFromKey(privKey)
	if err != nil {
		return nil, nil, errors.NotValidf("private key")
	}

	return sshPrivateKey, sshPrivateKey.PublicKey(), nil
}

func (tt *Tracker) machineHostKeys(ctx context.Context, req RequestArgs) ([]gossh.PublicKey, error) {
	stringHostKeys, err := tt.machines.MachineHostKeys(ctx, req.ModelUUID, req.MachineID)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get machine host key")
	}
	machineHostKeys := make([]gossh.PublicKey, len(stringHostKeys))

	// Machine host keys in the database are stored in openSSH's authorized_keys
	// format.
	for i, key := range stringHostKeys {
		machineHostKeys[i], _, _, _, err = gossh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return nil, errors.Annotate(err, "failed to parse machine host key")
		}
	}
	return machineHostKeys, nil
}

// RequestTunnel requests a tunnel to a model specific unit.
//
// The returned tunnelRequest should be used to wait for the tunnel to be established.
// See Wait() for more information.
func (tt *Tracker) RequestTunnel(ctx context.Context, req RequestArgs) (*gossh.Client, error) {
	tunnelID, err := uuid.NewUUID()
	if err != nil {
		return nil, err
	}

	// We use the same expiry for the password and the state entry.
	// The state's expiry is used to clean up any dangling requests.
	// The password expiry is used to invalidate old passwords.
	now := tt.clock.Now()
	ctx, cancel := context.WithDeadline(ctx, now.Add(maxTimeout))
	defer cancel()
	deadline, _ := ctx.Deadline()

	password, err := tt.authn.generatePassword(tunnelID.String(), now, deadline)
	if err != nil {
		return nil, err
	}

	privateKey, publicKey, err := tt.generateEphemeralSSHKey()
	if err != nil {
		return nil, err
	}

	controllerAddresses, err := tt.controller.Addresses(ctx)
	if err != nil {
		return nil, err
	}

	machineHostKeys, err := tt.machineHostKeys(ctx, req)
	if err != nil {
		return nil, err
	}

	// Make sure to use an unbuffered channel to ensure someone always
	// has responsibility of the connection passed around.
	connRecv := make(chan (net.Conn))

	tt.add(tunnelID.String(), connRecv)
	defer tt.delete(tunnelID.String())

	coreModelUUID := model.UUID(req.ModelUUID)
	if err := coreModelUUID.Validate(); err != nil {
		return nil, errors.Annotatef(err, "invalid model UUID %q", req.ModelUUID)
	}

	domainReq := domainssh.SSHConnRequest{
		TunnelID:            tunnelID.String(),
		MachineName:         req.MachineID,
		Expires:             deadline,
		SSHUsername:         reverseTunnelUser,
		SSHPassword:         password,
		ControllerAddresses: controllerAddresses,
		UnitPort:            0, // Allow the unit worker to determine the port.
		EphemeralPublicKey:  publicKey.Marshal(),
	}

	err = tt.connRequests.InsertSSHConnRequest(ctx, coreModelUUID, domainReq)
	if err != nil {
		return nil, err
	}

	return tt.wait(ctx, connRecv, privateKey, machineHostKeys)
}

func (tt *Tracker) add(tunnelID string, recv chan net.Conn) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.tracker[tunnelID] = recv
}

func (tt *Tracker) get(tunnelID string) (chan net.Conn, bool) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	req, ok := tt.tracker[tunnelID]
	return req, ok
}

func (tt *Tracker) delete(tunnelID string) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	delete(tt.tracker, tunnelID)
}

// AuthenticateTunnel authenticates an SSH request for a tunnel.
//
// An SSH server is expected to call this method to validate that
// the connection is a valid tunnel request.
//
// If the request is valid, the provided tunnelID should be
// stored and provided alongside the network connection to PushTunnel.
func (tt *Tracker) AuthenticateTunnel(username, password string) (tunnelID string, err error) {
	if username != reverseTunnelUser {
		return "", errors.New("invalid username")
	}

	return tt.authn.validatePassword(password)
}

// PushTunnel publishes a network connection for a tunnel.
// This method should only be called after AuthenticateTunnel
// which will provide the tunnelID.
//
// If an error is returned, e.g. because the tunnel ID is
// not valid, the caller should close the connection.
//
// If the error is nil, the caller should not close the connection.
//
// This method blocks unless a consumer is blocked waiting in a call
// to RequestTunnel(). Use context.WithTimeout to control the
// maximum time to wait.
func (tt *Tracker) PushTunnel(ctx context.Context, tunnelID string, conn net.Conn) error {
	recv, ok := tt.get(tunnelID)
	if !ok {
		return errors.New("tunnel not found")
	}
	select {
	case recv <- conn:
		return nil
	case <-ctx.Done():
		return errors.Annotate(ctx.Err(), "no one waiting for tunnel")
	}
}

// wait blocks until a TCP tunnel to the target unit is established.
//
// It is a mistake not to call Wait() after a successful call to RequestTunnel()
// as this will leak resources in the tunnel tracker.
// If the tunnel is no longer required, the caller should call Close() on the
// returned client.
//
// Use context.WithTimeout to control the maximum time to wait for the tunnel
// to be established.
func (tt *Tracker) wait(ctx context.Context, recv chan (net.Conn), privateKey gossh.Signer, hostKeys []gossh.PublicKey) (*gossh.Client, error) {
	select {
	case conn := <-recv:
		// We now have ownership of the connection, so we should close it
		// if the SSH dial fails.
		sshClient, err := tt.dialer.Dial(conn, defaultUser, privateKey, useFixedHostKeys(hostKeys))
		if err != nil {
			conn.Close()
			return nil, err
		}
		return sshClient, nil
	case <-ctx.Done():
		return nil, errors.Annotate(ctx.Err(), "waiting for tunnel")
	}
}

func useFixedHostKeys(keys []gossh.PublicKey) gossh.HostKeyCallback {
	hk := &fixedHostKeys{keys}
	return hk.check
}

type fixedHostKeys struct {
	keys []gossh.PublicKey
}

func (f *fixedHostKeys) check(hostname string, remote net.Addr, key gossh.PublicKey) error {
	for _, ourKey := range f.keys {
		if bytes.Equal(key.Marshal(), ourKey.Marshal()) {
			return nil
		}
	}
	return fmt.Errorf("ssh: host key mismatch")
}
