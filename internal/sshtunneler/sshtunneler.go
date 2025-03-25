// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"encoding/base64"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
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
	tunnelIDClaim     = "tunnelID"
)

// State writes requests for tunnels to state.
type State interface {
	InsertSSHConnRequest(arg state.SSHConnRequestArg) error
}

// ControllerInfo fetches the controller's address.
type ControllerInfo interface {
	Addresses() network.SpaceAddresses
}

// SSHDial establishes an SSH connection over the provided connection.
type SSHDial interface {
	Dial(conn net.Conn, username string, privateKey gossh.Signer) (*gossh.Client, error)
}

type tunnelAuthentication struct {
	sharedSecret []byte
	jwtAlg       jwa.KeyAlgorithm
	clock        clock.Clock
}

type tunnelRequest struct {
	privateKey gossh.Signer
	dialer     SSHDial
	recv       chan (net.Conn)
	cleanup    func()
}

type tunnelTracker struct {
	authn      tunnelAuthentication
	state      State
	controller ControllerInfo
	dialer     SSHDial
	mu         sync.Mutex
	tracker    map[string]*tunnelRequest
}

// TunnelTrackerArgs holds the arguments for creating a new tunnel tracker.
type TunnelTrackerArgs struct {
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

// NewTunnelTracker creates a new tunnel tracker.
// A tunnel tracker provides methods to create
// SSH tunnels to machine units.
func NewTunnelTracker(args TunnelTrackerArgs) (*tunnelTracker, error) {
	authn := tunnelAuthentication{
		jwtAlg:       args.JWTAlg,
		sharedSecret: args.SharedSecret,
		clock:        args.Clock,
	}

	return &tunnelTracker{
		tracker:    make(map[string]*tunnelRequest),
		authn:      authn,
		controller: args.ControllerInfo,
		state:      args.State,
		dialer:     args.Dialer,
	}, nil
}

// RequestArgs holds the arguments for requesting a tunnel.
type RequestArgs struct {
	unitName  string
	modelUUID string
}

func (tAuth *tunnelAuthentication) generatePassword(tunnelID string) (string, error) {
	token, err := jwt.NewBuilder().
		Issuer(tokenIssuer).
		Subject(tokenSubject).
		IssuedAt(tAuth.clock.Now()).
		Expiration(tAuth.clock.Now().Add(maxTimeout)).
		Claim(tunnelIDClaim, tunnelID).
		Build()
	if err != nil {
		return "", errors.Annotate(err, "failed to build token")
	}

	signedToken, err := jwt.Sign(token, jwt.WithKey(tAuth.jwtAlg, tAuth.sharedSecret))
	if err != nil {
		return "", errors.Annotate(err, "failed to sign token")
	}

	return base64.StdEncoding.EncodeToString(signedToken), nil
}

func (tAuth *tunnelAuthentication) validatePassword(password string) (string, error) {
	decodedToken, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return "", errors.Annotate(err, "failed to decode token")
	}

	token, err := jwt.Parse(decodedToken,
		jwt.WithKey(tAuth.jwtAlg, tAuth.sharedSecret),
		jwt.WithClock(tAuth.clock),
	)
	if err != nil {
		return "", errors.Annotate(err, "failed to parse token")
	}

	tunnelID, ok := token.PrivateClaims()[tunnelIDClaim].(string)
	if !ok {
		return "", errors.New("invalid token")
	}
	return tunnelID, nil
}

func (tt *tunnelTracker) generateEphemeralSSHKey() (gossh.Signer, gossh.PublicKey, error) {
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
func (tt *tunnelTracker) RequestTunnel(req RequestArgs) (*tunnelRequest, error) {
	tunnelID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	password, err := tt.authn.generatePassword(tunnelID.String())
	if err != nil {
		return nil, err
	}

	privateKey, publicKey, err := tt.generateEphemeralSSHKey()
	if err != nil {
		return nil, err
	}

	args := state.SSHConnRequestArg{
		TunnelID:           tunnelID.String(),
		ModelUUID:          req.modelUUID,
		UnitName:           req.unitName,
		Expires:            time.Now().Add(maxTimeout),
		Username:           reverseTunnelUser,
		Password:           password,
		ControllerAddress:  tt.controller.Addresses(),
		UnitPort:           22,
		EphemeralPublicKey: gossh.MarshalAuthorizedKey(publicKey),
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
	tunnelReq := &tunnelRequest{
		recv:       make(chan (net.Conn)),
		privateKey: privateKey,
		cleanup:    cleanup,
		dialer:     tt.dialer,
	}

	tt.add(tunnelID.String(), tunnelReq)

	return tunnelReq, nil
}

func (tt *tunnelTracker) add(tunnelID string, req *tunnelRequest) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	tt.tracker[tunnelID] = req
}

func (tt *tunnelTracker) get(tunnelID string) (*tunnelRequest, bool) {
	tt.mu.Lock()
	defer tt.mu.Unlock()
	req, ok := tt.tracker[tunnelID]
	return req, ok
}

func (tt *tunnelTracker) delete(tunnelID string) {
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
func (tt *tunnelTracker) AuthenticateTunnel(username, password string) (tunnelID string, err error) {
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
func (tt *tunnelTracker) PushTunnel(ctx context.Context, tunnelID string, conn net.Conn) error {
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
func (tr *tunnelRequest) Wait(ctx context.Context) (*gossh.Client, error) {
	defer tr.cleanup()
	select {
	case conn := <-tr.recv:
		return tr.dialer.Dial(conn, "ubuntu", tr.privateKey)
	case <-ctx.Done():
		return nil, errors.Annotate(ctx.Err(), "waiting for tunnel")
	}
}
