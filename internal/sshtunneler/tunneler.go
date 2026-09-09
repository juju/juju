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
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	domainssh "github.com/juju/juju/domain/ssh"
	"github.com/juju/juju/internal/pki/ssh"
	"github.com/juju/juju/internal/uuid"
)

var (
	maxTimeout = 60 * time.Second
)

const (
	defaultUser = "ubuntu"
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
	// WatchMachineHostKeys returns a watcher for SSH host key changes for the
	// given machine in the specified model. The watcher sends an initial event.
	WatchMachineHostKeys(ctx context.Context, modelUUID, machineID string) (watcher.StringsWatcher, error)
}

// ControllerInfo defines an interface to fetch the local controller node's
// addresses.
type ControllerInfo interface {
	// LocalAddresses returns the API addresses of the controller node
	// identified by controllerNodeID, for use as reverse-tunnel callback
	// candidates. Only the local node's addresses are returned so that the
	// machine's reverse tunnel connects back to the same node that is
	// holding the waiting client SSH session.
	LocalAddresses(ctx context.Context, controllerNodeID string) (network.SpaceAddresses, error)
}

// SSHDialer defines an interface to establish an SSH connection over a provided connection.
type SSHDial interface {
	Dial(conn net.Conn, username string, privateKey gossh.Signer, hostKeyCallback gossh.HostKeyCallback) (*gossh.Client, error)
}

// Tracker provides methods to create SSH tunnels to machine units.
// The tracker keeps track of consumers who have requested tunnels and allows
// an SSH server to push tunnels to these consumers. It is also a worker and
// must be stopped by callers that create it.
type Tracker struct {
	catacomb catacomb.Catacomb

	connReqState ConnRequestState
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

// NewTracker creates and starts a new tunnel tracker worker.
func NewTracker(args TrackerArgs) (*Tracker, error) {
	if err := args.validate(); err != nil {
		return nil, err
	}

	tt := &Tracker{
		tracker:      make(map[string]chan (net.Conn)),
		controller:   args.ControllerInfo,
		clock:        args.Clock,
		connReqState: args.ConnRequestState,
		machines:     args.MachineState,
		dialer:       args.Dialer,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "ssh-tunnel-tracker",
		Site: &tt.catacomb,
		Work: tt.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return tt, nil
}

func (tt *Tracker) loop() error {
	<-tt.catacomb.Dying()
	return tt.catacomb.ErrDying()
}

// Kill stops the tracker.
func (tt *Tracker) Kill() {
	tt.catacomb.Kill(nil)
}

// Wait blocks until the tracker has completed.
func (tt *Tracker) Wait() error {
	return tt.catacomb.Wait()
}

var _ worker.Worker = (*Tracker)(nil)

// RequestArgs holds the arguments for requesting a tunnel.
type RequestArgs struct {
	MachineID string
	ModelUUID string
	// ControllerNodeID is the ID of the local controller node that received
	// the SSH connection request. The reverse-tunnel callback address list
	// is restricted to this node's API addresses so the machine connects
	// back to the node that is already holding the waiting client session.
	ControllerNodeID string
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
// Use context.WithTimeout to control the maximum time to wait for the tunnel
// to be established.
func (tt *Tracker) RequestTunnel(ctx context.Context, req RequestArgs) (*gossh.Client, error) {
	ctx = tt.catacomb.Context(ctx)

	if req.MachineID == "" {
		return nil, errors.NotValidf("empty MachineID")
	}
	if req.ControllerNodeID == "" {
		return nil, errors.NotValidf("empty ControllerNodeID")
	}
	modelUUID := model.UUID(req.ModelUUID)
	if err := modelUUID.Validate(); err != nil {
		return nil, errors.Annotatef(err, "invalid model UUID %q", req.ModelUUID)
	}

	tunnelID, err := uuid.NewUUID()
	if err != nil {
		return nil, err
	}

	// The state entry's expiry is used to clean up any dangling
	// requests. Authentication of the machine now happens at the
	// HTTP layer on the tunnel upgrade endpoint, so no SSH
	// credentials are minted here.
	now := tt.clock.Now()
	ctx, cancel := context.WithDeadline(ctx, now.Add(maxTimeout))
	defer cancel()
	deadline, _ := ctx.Deadline()

	privateKey, publicKey, err := tt.generateEphemeralSSHKey()
	if err != nil {
		return nil, err
	}

	controllerAddresses, err := tt.controller.LocalAddresses(ctx, req.ControllerNodeID)
	if err != nil {
		return nil, err
	}

	// Make sure to use an unbuffered channel to ensure someone always
	// has responsibility of the connection passed around.
	connRecv := make(chan (net.Conn))

	tt.add(tunnelID.String(), connRecv)
	defer tt.delete(tunnelID.String())

	domainReq := domainssh.SSHConnRequest{
		TunnelID:            tunnelID.String(),
		MachineName:         req.MachineID,
		Expires:             deadline,
		ControllerAddresses: controllerAddresses,
		UnitPort:            0, // Allow the unit worker to determine the port.
		EphemeralPublicKey:  publicKey.Marshal(),
	}

	err = tt.connReqState.InsertSSHConnRequest(ctx, modelUUID, domainReq)
	if err != nil {
		return nil, err
	}

	return tt.wait(ctx, connRecv, privateKey, req)
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

// PushTunnel publishes a network connection for a tunnel.
// The tunnel ID must have been created by a call to RequestTunnel
// on this tracker; tunnel IDs from other controller nodes are
// rejected to preserve origin controller affinity.
//
// If an error is returned, e.g. because the tunnel ID is
// not valid, the caller should close the connection.
//
// If the error is nil, the caller should not close the connection.
//
// The returned channel is closed when the pushed connection is closed,
// so the caller can detect tunnel teardown without polling.
//
// This method blocks unless a consumer is blocked waiting in a call
// to RequestTunnel(). Use context.WithTimeout to control the
// maximum time to wait.
func (tt *Tracker) PushTunnel(ctx context.Context, tunnelID string, conn net.Conn) (<-chan struct{}, error) {
	ctx = tt.catacomb.Context(ctx)

	recv, ok := tt.get(tunnelID)
	if !ok {
		return nil, errors.New("tunnel not found")
	}
	done := make(chan struct{})
	wrapped := &closeNotifyConn{Conn: conn, done: done}
	select {
	case recv <- wrapped:
		return done, nil
	case <-ctx.Done():
		return nil, errors.Annotate(ctx.Err(), "no one waiting for tunnel")
	}
}

// closeNotifyConn wraps a net.Conn so that Close closes a done channel,
// letting PushTunnel callers detect teardown without polling.
type closeNotifyConn struct {
	net.Conn
	done chan struct{}
	once sync.Once
}

// Close closes the underlying connection and the done channel.
func (c *closeNotifyConn) Close() error {
	c.once.Do(func() { close(c.done) })
	return c.Conn.Close()
}

// wait blocks until a TCP tunnel to the target unit is established and the
// machine's SSH host keys are available. The context deadline bounds both
// waits.
func (tt *Tracker) wait(ctx context.Context, recv chan (net.Conn), privateKey gossh.Signer, req RequestArgs) (*gossh.Client, error) {
	select {
	case conn := <-recv:
		// We now have ownership of the connection, so we should close it
		// if the SSH dial fails.
		hostKeys, err := tt.machineHostKeysWithWatcher(ctx, req)
		if err != nil {
			conn.Close()
			return nil, err
		}
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

func (tt *Tracker) machineHostKeysWithWatcher(ctx context.Context, req RequestArgs) ([]gossh.PublicKey, error) {
	hostKeyWatcher, err := tt.machines.WatchMachineHostKeys(ctx, req.ModelUUID, req.MachineID)
	if err != nil {
		return nil, errors.Annotate(err, "watching for machine SSH host keys")
	}
	if err := tt.catacomb.Add(hostKeyWatcher); err != nil {
		return nil, errors.Annotate(err, "adding machine SSH host key watcher")
	}
	// Stop the watcher when we exit the scope of this function to avoid
	// accumulating watchers indefinitely.
	// We also add it to the catacomb to indicate that this worker
	// has resources that are pending cleanup.
	defer func() {
		hostKeyWatcher.Kill()
		_ = hostKeyWatcher.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, errors.Annotate(ctx.Err(), "waiting for machine SSH host keys")
		case _, ok := <-hostKeyWatcher.Changes():
			if !ok {
				return nil, errors.New("machine SSH host key watcher closed")
			}

			hostKeys, err := tt.machineHostKeys(ctx, req)
			if err != nil {
				return nil, err
			}
			if len(hostKeys) != 0 {
				return hostKeys, nil
			}
		}
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
