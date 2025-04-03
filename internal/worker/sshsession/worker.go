package sshsession

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/juju/errors"
	jujussh "github.com/juju/utils/v3/ssh"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

var (
	// ControllerSSHUser is the user that will connect to the controller's embedded
	// SSH server.
	ControllerSSHUser = "reverse-ssh"
	// We use authorized_keys2 as it's a default configured alternative file to
	// authorized_keys. Our existing SSH implementation was hardcoded against authorized_keys
	// and using this file allows us to not interfere with existing setups. The user may
	// still add another file to the "AuthorizedKeysFile" directive in sshd_config.
	//
	// TODO(ale8k): At somepoint, update juju documentation to explain should they wish
	// to ssh directly to a machine, this is the way to do so.
	AuthorizedKeysFile = "authorized_keys2"
)

// FacadeClient holds the facade methods for the SSH session worker.
type FacadeClient interface {
	// WatchSSHConnRequest creates a watcher and returns its ID for watching changes.
	WatchSSHConnRequest(machineId string) (watcher.StringsWatcher, error)

	// GetSSHConnRequest returns a ssh connection request by its connection request ID.
	GetSSHConnRequest(arg string) (params.SSHConnRequest, error)
}

// WorkerConfig encapsulates the configuration options for
// instantiating a new ssh session worker.
type WorkerConfig struct {
	Logger           Logger
	MachineId        string
	FacadeClient     FacadeClient
	ConnectionGetter ConnectionGetter
	KeyManager       KeyManager
}

// Validate checks whether the worker configuration settings are valid.
func (cfg WorkerConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.MachineId == "" {
		return errors.NotValidf("empty MachineId")
	}
	if cfg.FacadeClient == nil {
		return errors.NotValidf("nil FacadeClient")
	}
	if cfg.ConnectionGetter == nil {
		return errors.NotValidf("nil ConnectionGetter")
	}
	if cfg.KeyManager == nil {
		return errors.NotValidf("nil KeyManager")
	}

	return nil
}

// sshSessionWorker is a worker that enables SSH connections to a machine.
type sshSessionWorker struct {
	catacomb catacomb.Catacomb

	logger           Logger
	machineId        string
	facadeClient     FacadeClient
	connectionGetter ConnectionGetter
	keyManager       KeyManager
}

// NewWorker returns an SSH session worker.
func NewWorker(cfg WorkerConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &sshSessionWorker{
		logger:           cfg.Logger,
		machineId:        cfg.MachineId,
		facadeClient:     cfg.FacadeClient,
		connectionGetter: cfg.ConnectionGetter,
		keyManager:       cfg.KeyManager,
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})

	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// loop starts the workers main loop.
func (w *sshSessionWorker) loop() error {
	connRequestWatcher, err := w.facadeClient.WatchSSHConnRequest(w.machineId)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.catacomb.Add(connRequestWatcher); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes := <-connRequestWatcher.Changes():
			for _, connId := range changes {
				requestContext := w.catacomb.Context(context.Background())

				err := w.handleConnection(requestContext, connId)
				if err != nil {
					w.logger.Errorf("Failed to handle connection %q: %v", connId, err)
				}
			}
		}
	}
}

// Kill implements the worker.Worker interface.
func (w *sshSessionWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (w *sshSessionWorker) Wait() error {
	return w.catacomb.Wait()
}

// handleConnection handles initiating a reverse SSH connection to the controller
// and piping it to the local sshd of the machine agent's machine.
//
// This function does the following:
//  1. Gets the controllers address and ephemeral public key for the connection.
//  2. Verifies the address is known to this machine agent.
//  3. Adds the ephemeral public key to the authorized_keys2 file.
//  4. Dials the controllers SSH server expecting an SSH connection to come
//     back from the controller's SSH server.
//  5. Pipes the connection to the local sshd.
//  6. On connection close, removes the ephemeral public key from the authorized_keys2 file.
func (w *sshSessionWorker) handleConnection(ctx context.Context, connID string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		reqParams, err := w.facadeClient.GetSSHConnRequest(connID)
		if err != nil {
			return errors.Trace(err)
		}

		ctrlAddress := reqParams.ControllerAddresses.Values()[0]
		ephemeralPublicKey := string(reqParams.EphemeralPublicKey)

		if err := w.keyManager.AddPublicKey(ephemeralPublicKey); err != nil {
			return errors.Trace(err)
		}

		defer func() {
			if err := w.keyManager.CleanupPublicKey(ephemeralPublicKey); err != nil {
				w.logger.Errorf("Error cleaning up ephemeral public key: %v", err)
			}
		}()

		if err := w.pipeConnectionToSSHD(ctx, ctrlAddress, reqParams.Password); err != nil {
			return errors.Trace(err)
		}

		return nil
	}
}

// pipeConnectionToSSHD initiates the connection back to the controller and pipes
// it over to the local SSHD. This call blocks until the connection has finished.
func (w *sshSessionWorker) pipeConnectionToSSHD(ctx context.Context, ctrlAddress, password string) error {
	controllerConn, err := w.connectionGetter.GetControllerConnection(password, ctrlAddress)
	if err != nil {
		return errors.Trace(err)
	}
	defer controllerConn.Close()

	sshdConn, err := w.connectionGetter.GetSSHDConnection()
	if err != nil {
		return errors.Trace(err)
	}
	defer sshdConn.Close()

	cancellableControllerConnection := newCancellableReadWriter(ctx, controllerConn)
	cancellableSSHDConnection := newCancellableReadWriter(ctx, sshdConn)
	eg := errgroup.Group{}

	eg.Go(func() error {
		// sshd -> conn
		_, err := io.Copy(cancellableControllerConnection, cancellableSSHDConnection)
		if err != nil {
			return err
		}
		return nil
	})

	eg.Go(func() error {
		// conn -> sshd
		_, err = io.Copy(cancellableSSHDConnection, cancellableControllerConnection)
		if err != nil {
			return err
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// KeyManager holds the methods necessary to add/cleanup public keys.
type KeyManager interface {
	AddPublicKey(ephemeralPublicKey string) error
	CleanupPublicKey(ephemeralPublicKey string) error
}

// keyManager handles the addition and removal of public keys to the authorized_keys2 file.
type keyManager struct {
	logger Logger
}

// NewKeyManager returns a new keyManager.
func NewKeyManager(l Logger) *keyManager {
	return &keyManager{l}
}

// AddPublicKey adds the provided public key to the authorized_keys2 file.
func (w *keyManager) AddPublicKey(ephemeralPublicKey string) error {
	if err := jujussh.AddKeysToFile(ControllerSSHUser, AuthorizedKeysFile, []string{ephemeralPublicKey}); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// CleanupPublicKey finds the fingerprint of the provided key and attempts to delete the key
// from authorized_keys2.
func (w *keyManager) CleanupPublicKey(ephemeralPublicKey string) error {
	fingerprint, _, err := jujussh.KeyFingerprint(ephemeralPublicKey)
	if err != nil {
		return errors.Trace(err)
	}

	if err := jujussh.DeleteKeysFromFile(ControllerSSHUser, AuthorizedKeysFile, []string{fingerprint}); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// ConnectionGetter provides the methods to connect to a controller and the local SSHD.
type ConnectionGetter interface {
	GetControllerConnection(password, ctrlAddress string) (ssh.Channel, error)
	GetSSHDConnection() (net.Conn, error)
}

// connectionGetter is capable of initating SSH connections to two places.
//
//  1. The controller's SSH server
//  2. The local SSHD on the machine
//
// The consumer is expect to pipe these connections together.
type connectionGetter struct {
	logger Logger
}

// NewConnectionGetter returns a new connectionGetter.
func NewConnectionGetter(l Logger) *connectionGetter {
	return &connectionGetter{l}
}

// GetControllerConnection initiates an SSH connection to the target ctrlAddress.
func (w *connectionGetter) GetControllerConnection(password, ctrlAddress string) (ssh.Channel, error) {
	// TODO(ale8k): Watch will return host key in subsequent PR.
	sshConfig := &ssh.ClientConfig{
		User:            ControllerSSHUser,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO(ale8k): Fill in host key here
	}

	client, err := ssh.Dial("tcp", ctrlAddress, sshConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO(ale8k): Make this a constant that both the server and session worker can use.
	ch, in, err := client.OpenChannel("juju-tunnel", nil)
	if err != nil {
		return nil, err
	}
	go ssh.DiscardRequests(in)

	return ch, nil
}

// GetSSHConnection performs a stand TCP dial to the SSHD running on the machine.
func (w *connectionGetter) GetSSHDConnection() (net.Conn, error) {
	etcPath := "/etc/ssh/sshd_config"
	openSSHPath := "/usr/share/openssh/sshd_config"
	port := w.getLocalSSHPort(etcPath, openSSHPath)
	u, err := url.Parse("localhost" + port)
	if err != nil {
		return nil, errors.Trace(err)
	}

	localSSHD, err := net.Dial("tcp", u.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return localSSHD, nil
}

// getLocalSSHPort parses the local sshd_config file to get the port sshd is listening on.
// It will try all the provided paths.
// If it cannot get it for any reason, it'll log the error and return the default port 22.
func (w *connectionGetter) getLocalSSHPort(filePaths ...string) string {
	port := "22"

	for _, filePath := range filePaths {
		file, err := os.Open(filePath)
		if err != nil {
			w.logger.Errorf("Error opening sshd_config file: %v", err)
			return port
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			if strings.HasPrefix(line, "#") || line == "" {
				continue
			}

			if strings.HasPrefix(line, "Port") {
				fields := strings.Fields(line)
				if len(fields) == 2 {
					port = fields[1]
				}
				break
			}
		}

		if err := scanner.Err(); err != nil {
			w.logger.Errorf("Error reading sshd_config file: %v", err)
			return port
		}
		return port
	}

	return port
}

// cancellableReadWriter provides a means to cancel a read or write operation via context.
type cancellableReadWriter struct {
	ctx context.Context
	rw  io.ReadWriter
}

// newCancellableReadWriter returns a new cancellableReadWriter.
func newCancellableReadWriter(ctx context.Context, rw io.ReadWriter) *cancellableReadWriter {
	return &cancellableReadWriter{
		ctx: ctx,
		rw:  rw,
	}
}

// Implements io.ReadWriter.
func (crw *cancellableReadWriter) Read(p []byte) (int, error) {
	select {
	case <-crw.ctx.Done():
		return 0, crw.ctx.Err()
	default:
		return crw.rw.Read(p)
	}
}

// Implements io.ReadWriter.
func (crw *cancellableReadWriter) Write(p []byte) (int, error) {
	select {
	case <-crw.ctx.Done():
		return 0, crw.ctx.Err()
	default:
		return crw.rw.Write(p)
	}
}
