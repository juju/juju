// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/juju/errors"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/pki/ssh"
	"github.com/juju/juju/state"
)

const (
	maxTimeout = 60 * time.Second
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

type tunnelRequest struct {
	privateKey gossh.Signer
	dialer     SSHDial
	recv       chan (net.Conn)
	cleanup    func()
}

type tunnelTracker struct {
	sharedSecret []byte
	jwtAlg       jwa.KeyAlgorithm
	state        State
	controller   ControllerInfo
	dialer       SSHDial
	mu           sync.Mutex
	tracker      map[string]*tunnelRequest
}

// NewTunnelTracker creates a new tunnel tracker.
// A tunnel tracker provides methods to create
// SSH tunnels to machine units.
func NewTunnelTracker(state State, controllerInfo ControllerInfo, dialer SSHDial) (*tunnelTracker, error) {
	// The shared secret is generated dynamically because
	// user's SSH connections to the controller only live
	// while the controller is running.
	// So a restart of the controller, and a new key is totally okay.
	key := make([]byte, 64) // 64 bytes for HS512
	if _, err := rand.Read(key); err != nil {
		return nil, errors.Annotate(err, "failed to generate shared secret")
	}

	return &tunnelTracker{
		tracker:      make(map[string]*tunnelRequest),
		jwtAlg:       jwa.HS512,
		sharedSecret: key,
		controller:   controllerInfo,
		state:        state,
		dialer:       dialer,
	}, nil
}

// RequestArgs holds the arguments for requesting a tunnel.
type RequestArgs struct {
	unitName  string
	modelUUID string
}

func (t *tunnelTracker) generateBase64JWT(tunnelID string) (string, error) {
	token, err := jwt.NewBuilder().
		Issuer("sshtunneler").
		Subject("reverse-tunnel").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(maxTimeout)).
		Claim("tunnelID", tunnelID).
		Build()
	if err != nil {
		return "", errors.Annotate(err, "failed to build token")
	}

	signedToken, err := jwt.Sign(token, jwt.WithKey(t.jwtAlg, t.sharedSecret))
	if err != nil {
		return "", errors.Annotate(err, "failed to sign token")
	}

	return base64.StdEncoding.EncodeToString(signedToken), nil
}

func (t *tunnelTracker) generateEphemeralSSHKey() (gossh.Signer, gossh.PublicKey, error) {
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
func (t *tunnelTracker) RequestTunnel(req RequestArgs) (*tunnelRequest, error) {
	tunnelID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	jwtPassword, err := t.generateBase64JWT(tunnelID.String())
	if err != nil {
		return nil, err
	}

	privateKey, publicKey, err := t.generateEphemeralSSHKey()
	if err != nil {
		return nil, err
	}

	args := state.SSHConnRequestArg{
		TunnelID:           tunnelID.String(),
		ModelUUID:          req.modelUUID,
		UnitName:           req.unitName,
		Expires:            time.Now().Add(maxTimeout),
		Username:           "reverse-tunnel",
		Password:           jwtPassword,
		ControllerAddress:  t.controller.Addresses(),
		UnitPort:           22,
		EphemeralPublicKey: gossh.MarshalAuthorizedKey(publicKey),
	}

	err = t.state.InsertSSHConnRequest(args)
	if err != nil {
		return nil, err
	}

	cleanup := func() {
		t.delete(tunnelID.String())
	}
	// Make sure to use an unbuffered channel to ensure someone always
	// has responsibility of the connection passed around.
	tunnelReq := &tunnelRequest{
		recv:       make(chan (net.Conn)),
		privateKey: privateKey,
		cleanup:    cleanup,
		dialer:     t.dialer,
	}

	t.mu.Lock()
	t.tracker[tunnelID.String()] = tunnelReq
	t.mu.Unlock()

	return tunnelReq, nil
}

func (t *tunnelTracker) getTunnel(tunnelID string) (*tunnelRequest, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	req, ok := t.tracker[tunnelID]
	return req, ok
}

func (t *tunnelTracker) delete(tunnelID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tracker, tunnelID)
}

// AuthenticateTunnel authenticates an SSH request for a tunnel.
//
// An SSH server is expected to call this method to validate that
// the connection is a valid tunnel request.
//
// If the request is valid, the provided tunnelID should be
// stored and provided alongside the network connection to PushTunnel.
func (t *tunnelTracker) AuthenticateTunnel(username, password string) (tunnelID string, err error) {
	if username != "reverse-tunnel" {
		return "", errors.New("invalid username")
	}

	decodedToken, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return "", errors.Annotate(err, "failed to decode token")
	}

	token, err := jwt.Parse(decodedToken, jwt.WithKey(t.jwtAlg, t.sharedSecret))
	if err != nil {
		return "", errors.Annotate(err, "failed to parse token")
	}

	tunnelID, ok := token.PrivateClaims()["tunnelID"].(string)
	if !ok {
		return "", errors.New("invalid token")
	}
	return tunnelID, nil
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
func (t *tunnelTracker) PushTunnel(ctx context.Context, tunnelID string, conn net.Conn) error {
	req, ok := t.getTunnel(tunnelID)
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
func (t *tunnelRequest) Wait(ctx context.Context) (*gossh.Client, error) {
	defer t.cleanup()
	select {
	case conn := <-t.recv:
		return t.dialer.Dial(conn, "ubuntu", t.privateKey)
	case <-ctx.Done():
		return nil, errors.Annotate(ctx.Err(), "waiting for tunnel")
	}
}
