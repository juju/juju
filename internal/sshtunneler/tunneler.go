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

	"github.com/google/uuid"
	"github.com/juju/errors"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/pki/ssh"
	"github.com/juju/juju/state"
)

var (
	maxTimeout = 60 * time.Second
)

const (
	// ReverseTunnelUser is the user name unit agents use to connect to
	// the controller.
	ReverseTunnelUser = "juju-reverse-tunnel"
	JujuTunnelChannel = "juju-tunnel"
	tokenIssuer       = "sshtunneler"
	tokenSubject      = "reverse-tunnel"
	tunnelIDClaimKey  = "tunnelID"
	defaultUser       = "ubuntu"
)

// State defines an interface to write requests for tunnels to state.
type State interface {
	InsertSSHConnRequest(arg state.SSHConnRequestArg) error
	MachineHostKeys(modelUUID, machineID string) ([]string, error)
}

// ControllerInfo defines an interface to fetch the controller's address.
type ControllerInfo interface {
	Addresses() (network.SpaceAddresses, error)
}

// SSHDialer defines an interface to establish an SSH connection over a provided connection.
type SSHDial interface {
	Dial(conn net.Conn, username string, privateKey gossh.Signer, hostKeyCallback gossh.HostKeyCallback) (*gossh.Client, error)
}

// Clock defines an interface for getting the current time.
type Clock interface {
	Now() time.Time
}

// Tracker provides methods to create SSH tunnels to machine units.
// The objects keep track of consumers who have requested tunnels
// and allows an SSH server to push tunnels to these consumers.
type Tracker struct {
	authn      tunnelAuthentication
	state      State
	controller ControllerInfo
	dialer     SSHDial
	clock      Clock

	mu      sync.Mutex
	tracker map[string]chan (net.Conn)
}

// TrackerArgs holds the arguments for creating a new tunnel tracker.
type TrackerArgs struct {
	State          State
	ControllerInfo ControllerInfo
	Dialer         SSHDial
	Clock          Clock
}

func (args *TrackerArgs) validate() error {
	if args.State == nil {
		return errors.New("state is required")
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
		tracker:    make(map[string]chan (net.Conn)),
		authn:      authn,
		controller: args.ControllerInfo,
		clock:      args.Clock,
		state:      args.State,
		dialer:     args.Dialer,
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

func (tt *Tracker) machineHostKeys(req RequestArgs) ([]gossh.PublicKey, error) {
	stringHostKeys, err := tt.state.MachineHostKeys(req.ModelUUID, req.MachineID)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get machine host key")
	}
	machineHostKeys := make([]gossh.PublicKey, len(stringHostKeys))

	// Machine host keys in Mongo are stored in openSSH's authorized_keys format.
	for i, key := range stringHostKeys {
		machineHostKeys[i], _, _, _, err = gossh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return nil, errors.Annotate(err, "failed to parse machine host key")
		}
	}
	return machineHostKeys, nil
}

// RequestTunnel establishes an SSH connection to a model specific unit.
//
// This method will block until the tunnel is established or the context
// is cancelled or a maximum timeout of `maxTimeout` is reached.
func (tt *Tracker) RequestTunnel(ctx context.Context, req RequestArgs) (*gossh.Client, error) {
	tunnelID, err := uuid.NewRandom()
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

	controllerAddresses, err := tt.controller.Addresses()
	if err != nil {
		return nil, err
	}

	machineHostKeys, err := tt.machineHostKeys(req)
	if err != nil {
		return nil, err
	}

	// Make sure to use an unbuffered channel to ensure someone always
	// has responsibility of the connection passed around.
	connRecv := make(chan (net.Conn))

	tt.add(tunnelID.String(), connRecv)
	defer tt.delete(tunnelID.String())

	args := state.SSHConnRequestArg{
		TunnelID:            tunnelID.String(),
		ModelUUID:           req.ModelUUID,
		MachineId:           req.MachineID,
		Expires:             deadline,
		Username:            ReverseTunnelUser,
		Password:            password,
		ControllerAddresses: controllerAddresses,
		UnitPort:            0, // Allow the unit worker to determine the port.
		EphemeralPublicKey:  publicKey.Marshal(),
	}

	err = tt.state.InsertSSHConnRequest(args)
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
	if username != ReverseTunnelUser {
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
