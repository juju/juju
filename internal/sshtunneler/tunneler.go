// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwa"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/pki/ssh"
	"github.com/juju/juju/state"
)

const (
	maxTimeout        = 60 * time.Second
	reverseTunnelUser = "reverse-tunnel"
	tokenIssuer       = "sshtunneler"
	tokenSubject      = "reverse-tunnel"
	tunnelIDClaimKey  = "tunnelID"
	defaultUser       = "ubuntu"
)

// State defines an interface to write requests for tunnels to state.
type State interface {
	InsertSSHConnRequest(arg state.SSHConnRequestArg) error
}

// ControllerInfo defines an interface to fetch the controller's address.
type ControllerInfo interface {
	Addresses() network.SpaceAddresses
}

// SSHDialer defines an interface to establish an SSH connection over a provided connection.
type SSHDial interface {
	Dial(conn net.Conn, username string, privateKey gossh.Signer) (*gossh.Client, error)
}

// Request tracks a request for an SSH connection to
// a machine. See its Wait() method for more details.
type Request struct {
	privateKey gossh.Signer
	dialer     SSHDial
	recv       chan (net.Conn)
	cleanup    func()
}

// Tracker provides methods to create SSH tunnels to machine units.
// The objects keep track of consumers who have requested tunnels
// and allows an SSH server to push tunnels to these consumers.
type Tracker struct {
	authn      tunnelAuthentication
	state      State
	controller ControllerInfo
	dialer     SSHDial
	clock      clock.Clock

	mu      sync.Mutex
	tracker map[string]*Request
}

// TrackerArgs holds the arguments for creating a new tunnel tracker.
type TrackerArgs struct {
	State          State
	ControllerInfo ControllerInfo
	Dialer         SSHDial
	Clock          clock.Clock

	// SharedSecret is the secret used to sign and validate JWTs.
	SharedSecret []byte
	// JWTAlg is the algorithm used to sign JWTs and should match
	// the strength (number of bytes) of the SharedSecret.
	JWTAlg jwa.KeyAlgorithm
}

// NewTracker creates a new tunnel tracker.
func NewTracker(args TrackerArgs) (*Tracker, error) {
	if args.State == nil {
		return nil, errors.New("state is required")
	}
	if args.ControllerInfo == nil {
		return nil, errors.New("controller info is required")
	}
	if args.Dialer == nil {
		return nil, errors.New("dialer is required")
	}
	if args.Clock == nil {
		return nil, errors.New("clock is required")
	}
	if args.SharedSecret == nil {
		return nil, errors.New("shared secret is required")
	}

	authn := tunnelAuthentication{
		jwtAlg:       args.JWTAlg,
		sharedSecret: args.SharedSecret,
		clock:        args.Clock,
	}

	return &Tracker{
		tracker:    make(map[string]*Request),
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

// RequestTunnel requests a tunnel to a model specific unit.
//
// The returned tunnelRequest should be used to wait for the tunnel to be established.
// See Wait() for more information.
func (tt *Tracker) RequestTunnel(req RequestArgs) (*Request, error) {
	tunnelID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	// We use the same expiry for the password and the state entry.
	// The state's expiry is used to clean up any dangling requests.
	// The password expiry is used to invalidate old passwords.
	expiry := tt.clock.Now().Add(maxTimeout)

	password, err := tt.authn.generatePassword(tunnelID.String(), expiry)
	if err != nil {
		return nil, err
	}

	privateKey, publicKey, err := tt.generateEphemeralSSHKey()
	if err != nil {
		return nil, err
	}

	args := state.SSHConnRequestArg{
		TunnelID:            tunnelID.String(),
		ModelUUID:           req.ModelUUID,
		MachineId:           req.MachineID,
		Expires:             expiry,
		Username:            reverseTunnelUser,
		Password:            password,
		ControllerAddresses: tt.controller.Addresses(),
		UnitPort:            22,
		EphemeralPublicKey:  publicKey.Marshal(),
	}

	err = tt.state.InsertSSHConnRequest(args)
	if err != nil {
		return nil, err
	}

	cleanup := func() {
		tt.delete(tunnelID.String())
	}
	// Make sure to use an unbuffered channel to ensure someone always
	// has responsibility of the connection passed around.
	tunnelReq := &Request{
		recv:       make(chan (net.Conn)),
		privateKey: privateKey,
		cleanup:    cleanup,
		dialer:     tt.dialer,
	}

	tt.add(tunnelID.String(), tunnelReq)

	return tunnelReq, nil
}

func (tt *Tracker) add(tunnelID string, req *Request) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.tracker[tunnelID] = req
}

func (tt *Tracker) get(tunnelID string) (*Request, bool) {
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
// If the tunnelID is not valid an error is returned and the
// caller should close the connection.
//
// This method blocks until a consumer runs Wait() on the
// appropriate tunnel request. Use context.WithTimeout to control
// the maximum time to wait.
func (tt *Tracker) PushTunnel(ctx context.Context, tunnelID string, conn net.Conn) error {
	req, ok := tt.get(tunnelID)
	if !ok {
		return errors.New("tunnel not found")
	}
	select {
	case req.recv <- conn:
		return nil
	case <-ctx.Done():
		return errors.Annotate(ctx.Err(), "no one waiting for tunnel")
	}
}

// Wait blocks until a TCP tunnel to the target unit is established.
//
// It is a mistake not to call Wait() after a successful call to RequestTunnel()
// as this will leak resources in the tunnel tracker.
// If the tunnel is no longer required, the caller should call Close() on the
// returned client.
//
// Use context.WithTimeout to control the maximum time to wait for the tunnel
// to be established.
func (r *Request) Wait(ctx context.Context) (*gossh.Client, error) {
	defer r.cleanup()
	select {
	case conn := <-r.recv:
		// We now have ownership of the connection, so we should close it
		// if the SSH dial fails.
		sshClient, err := r.dialer.Dial(conn, defaultUser, r.privateKey)
		if err != nil {
			conn.Close()
			return nil, err
		}
		return sshClient, nil
	case <-ctx.Done():
		return nil, errors.Annotate(ctx.Err(), "waiting for tunnel")
	}
}
