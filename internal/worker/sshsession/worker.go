package sshsession

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/juju/errors"
	jujussh "github.com/juju/utils/v3/ssh"
	"github.com/juju/worker/v3"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sync/errgroup"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/watcher"
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
	// WatchSSHConnRequest returns a watcher that will return doc ids for
	// incoming SSH connection requests. These doc ids are to retrieve connect
	// params from GetSSHConnRequest(docID string) (SSHConnRequest, error).
	WatchSSHConnRequest(machineId string) (watcher.StringsWatcher, error)

	// GetSSHConnRequest returns the SSH connection request for the given docID.
	GetSSHConnRequest(docID string) (DummyParams, error)
}

// WorkerConfig encapsulates the configuration options for
// instantiating a new lease ssh session worker.
type WorkerConfig struct {
	Logger           Logger
	FacadeClient     FacadeClient
	Agent            agent.Agent
	ConnectionGetter ConnectionGetter
}

// Validate checks whether the worker configuration settings are valid.
func (cfg WorkerConfig) Validate() error {
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.FacadeClient == nil {
		return errors.NotValidf("nil FacadeClient")
	}
	if cfg.Agent == nil {
		return errors.NotValidf("nil Agent")
	}
	if cfg.ConnectionGetter == nil {
		return errors.NotValidf("nil ConnectionGetter")
	}

	return nil
}

// sshSessionWorker is a worker that enables SSH connections to a machine.
// It watches for incoming SSH connections to the controller's SSH server over
// a facade, and when notified initiates a connection to the controller's SSH server.
// When the controller's SSH server receives this connection, it initiates its own
// SSH connection down the tunnel and the sshSessionWorker pipes it to the SSHD
// on the machine.
type sshSessionWorker struct {
	tomb tomb.Tomb

	logger           Logger
	facadeClient     FacadeClient
	agent            agent.Agent
	connectionGetter ConnectionGetter
}

// NewWorker returns an SSH session worker.
func NewWorker(cfg WorkerConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &sshSessionWorker{
		logger:           cfg.Logger,
		facadeClient:     cfg.FacadeClient,
		agent:            cfg.Agent,
		connectionGetter: cfg.ConnectionGetter,
	}

	w.tomb.Go(w.loop)
	return w, nil
}

// loop starts the workers main loop. It watches for incoming SSH connection requests
// and handles them by initiating a reverse SSH connection to the controller and piping
// it to the local sshd of the machine agent's machine.
func (w *sshSessionWorker) loop() error {
	machineId := w.agent.CurrentConfig().Tag().Id()
	sw, err := w.facadeClient.WatchSSHConnRequest(machineId)
	if err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case changes := <-sw.Changes():
			for _, docID := range changes {
				err := w.handleConnection(docID)
				if err != nil {
					w.logger.Errorf("failed to handle connection %q: %v", docID, err)
				}
			}
		}
	}
}

// Kill implements the worker.Worker interface.
func (w *sshSessionWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (w *sshSessionWorker) Wait() error {
	return w.tomb.Wait()
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
func (w *sshSessionWorker) handleConnection(docID string) error {
	reqParams, err := w.facadeClient.GetSSHConnRequest(docID)
	if err != nil {
		w.logger.Errorf("failed to get SSH connection request: %v", err)
		return errors.Trace(err)
	}

	ctrlAddress := reqParams.ControllerAddress.Values()[0]
	ephemeralPublicKey := string(reqParams.EphemeralPublicKey)

	if err := w.verifyControllerAddress(ctrlAddress); err != nil {
		return errors.Trace(err)
	}

	if err = jujussh.AddKeys(ControllerSSHUser, AuthorizedKeysFile, ephemeralPublicKey); err != nil {
		w.logger.Errorf("failed to add ephemeral public key to %s: %v", AuthorizedKeysFile, err)
		return errors.Trace(err)
	}

	defer func() {
		if err := w.cleanupPublicKey(ephemeralPublicKey); err != nil {
			w.logger.Errorf("Error cleaning up ephemeral public key: %v", err)
		}
	}()

	if err := w.pipeConnectionToSSHD(ctrlAddress, reqParams.Password); err != nil {
		w.logger.Errorf("Error piping connection: %v", err)
		return errors.Trace(err)
	}

	return nil
}

// verifyControllerAddress verifies that the provided address is known to the agent configuration.
func (w *sshSessionWorker) verifyControllerAddress(ctrlAddress string) error {
	knownAddresses, err := w.agent.CurrentConfig().APIAddresses()
	if err != nil {
		w.logger.Errorf("failed to get api addresses: %v", err)
		return errors.Trace(err)
	}

	if !slices.Contains(knownAddresses, ctrlAddress) {
		msg := fmt.Sprintf("controller address %q not in known addresses %v", ctrlAddress, knownAddresses)
		w.logger.Errorf("controller address %q not in known addresses %v", ctrlAddress, knownAddresses)
		return errors.New(msg)
	}

	return nil
}

// pipeConnectionToSSHD initiates the connection back to the controller and pipes
// it over to the local SSHD. This call blocks until the connection has finished.
func (w *sshSessionWorker) pipeConnectionToSSHD(ctrlAddress, password string) error {
	conn, err := w.connectionGetter.GetSSHConnection(password, ctrlAddress)
	if err != nil {
		return errors.Trace(err)
	}
	defer conn.Close()

	sshdConn, err := w.connectionGetter.GetSSHDConnection()
	if err != nil {
		return errors.Trace(err)
	}
	defer sshdConn.Close()

	eg := errgroup.Group{}

	eg.Go(func() error {
		// sshd -> conn
		_, err := io.Copy(conn, sshdConn)
		if err != nil {
			return err
		}
		return nil
	})

	eg.Go(func() error {
		// conn -> sshd
		_, err = io.Copy(sshdConn, conn)
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

// cleanupPublicKey finds the fingerprint of the provided key and attempts to delete the key
// from authorized_keys2.
func (w *sshSessionWorker) cleanupPublicKey(ephemeralPublicKey string) error {
	fingerprint, _, err := jujussh.KeyFingerprint(ephemeralPublicKey)
	if err != nil {
		w.logger.Errorf("failed to get fingerprint of ephemeral public key: %v", err)
		return errors.Trace(err)
	}

	if err := jujussh.DeleteKeys(ControllerSSHUser, AuthorizedKeysFile, true, fingerprint); err != nil {
		w.logger.Errorf("Error deleting ephemeral public key: %v", err)
		return errors.Trace(err)
	}

	return nil
}

type ConnectionGetter interface {
	GetSSHConnection(password, ctrlAddress string) (net.Conn, error)
	GetSSHDConnection() (net.Conn, error)
}

type connectionGetter struct {
	logger Logger
}

// NewConnectionGetter returns a struct capable of initating SSH connections to two places.
//
//  1. The controller's SSH server
//  2. The local SSHD on the machine
//
// The consumer is expect to pipe these connections together.
func NewConnectionGetter(l Logger) *connectionGetter {
	return &connectionGetter{l}
}

// GetSSHConnection initiates an SSH connection to the target ctrlAddress.
func (w *connectionGetter) GetSSHConnection(password, ctrlAddress string) (net.Conn, error) {
	// TODO(ale8k): Watch will return host key in subsequent PR.
	sshConfig := &ssh.ClientConfig{
		User:            ControllerSSHUser,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO(ale8k): Fill in host key here
	}

	client, err := ssh.Dial("tcp", ctrlAddress, sshConfig)
	if err != nil {
		w.logger.Errorf("failed to dial controller %q: %v", ctrlAddress, err)
		return nil, errors.Trace(err)
	}

	conn, err := client.Dial("tcp", ctrlAddress)
	if err != nil {
		w.logger.Errorf("failed to dial controller %q: %v", ctrlAddress, err)
		return nil, errors.Trace(err)
	}

	return conn, nil
}

// GetSSHConnection performs a stand TCP dial to the SSHD running on the machine.
func (w *connectionGetter) GetSSHDConnection() (net.Conn, error) {
	etcPath := "/etc/ssh/sshd_config"
	openSSHPath := "/usr/share/openssh/sshd_config"
	port := w.getLocalSSHPort(etcPath, openSSHPath)
	u, err := url.Parse("localhost" + port)
	if err != nil {
		w.logger.Errorf("failed to parse local sshd port: %v", err)
		return nil, errors.Trace(err)
	}

	localSSHD, err := net.Dial("tcp", u.String())
	if err != nil {
		w.logger.Errorf("Failed to connect to local sshd: %v", err)
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
