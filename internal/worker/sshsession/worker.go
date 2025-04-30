// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/sshconn"
	"github.com/juju/juju/internal/sshtunneler"
	"github.com/juju/juju/internal/worker/authenticationworker"
	"github.com/juju/juju/rpc/params"
)

// FacadeClient holds the facade methods for the SSH session worker.
type FacadeClient interface {
	// WatchSSHConnRequest creates a watcher and returns its ID for watching changes.
	WatchSSHConnRequest(machineId string) (watcher.StringsWatcher, error)

	// GetSSHConnRequest returns a ssh connection request by its connection request ID.
	GetSSHConnRequest(arg string) (params.SSHConnRequest, error)

	// ControllerSSHPort returns the SSH port of the controller.
	ControllerSSHPort() (string, error)
}

// WorkerConfig encapsulates the configuration options for
// instantiating a new ssh session worker.
type WorkerConfig struct {
	Logger               Logger
	MachineId            string
	FacadeClient         FacadeClient
	ConnectionGetter     ConnectionGetter
	EphemeralKeysUpdater authenticationworker.EphemeralKeysUpdater
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
	if cfg.EphemeralKeysUpdater == nil {
		return errors.NotValidf("nil EphemeralKeysUpdater")
	}

	return nil
}

// sshSessionWorker is a worker that enables SSH connections to a machine.
type sshSessionWorker struct {
	catacomb catacomb.Catacomb

	logger               Logger
	machineId            string
	facadeClient         FacadeClient
	connectionGetter     ConnectionGetter
	ephemeralKeysUpdater authenticationworker.EphemeralKeysUpdater
}

// NewWorker returns an SSH session worker.
func NewWorker(cfg WorkerConfig) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &sshSessionWorker{
		logger:               cfg.Logger,
		machineId:            cfg.MachineId,
		facadeClient:         cfg.FacadeClient,
		connectionGetter:     cfg.ConnectionGetter,
		ephemeralKeysUpdater: cfg.EphemeralKeysUpdater,
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

	controllerSSHPort, err := w.facadeClient.ControllerSSHPort()
	if err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes := <-connRequestWatcher.Changes():
			for _, connId := range changes {
				go func() {
					requestContext := w.catacomb.Context(context.Background())
					err := w.handleConnection(requestContext, connId, controllerSSHPort)
					if err != nil {
						w.logger.Errorf("Failed to handle connection %q: %v", connId, err)
					}
				}()
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
//  3. Adds the ephemeral public key.
//  4. Dials the controllers SSH server expecting an SSH connection to come
//     back from the controller's SSH server.
//  5. Pipes the connection to the local sshd.
//  6. On connection close, removes the ephemeral public key.
func (w *sshSessionWorker) handleConnection(ctx context.Context, connID, ctrlPort string) error {
	w.logger.Errorf("Handling connection %q", connID)
	reqParams, err := w.facadeClient.GetSSHConnRequest(connID)
	if err != nil {
		return errors.Trace(err)
	}

	ctrlAddress := reqParams.ControllerAddresses.Values()[0]

	ephemeralPublicKey, err := ssh.ParsePublicKey(reqParams.EphemeralPublicKey)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.ephemeralKeysUpdater.AddEphemeralKey(ephemeralPublicKey, connID); err != nil {
		return errors.Trace(err)
	}

	defer func() {
		if err := w.ephemeralKeysUpdater.RemoveEphemeralKey(ephemeralPublicKey); err != nil {
			w.logger.Errorf("Error cleaning up ephemeral public key: %v", err)
		}
	}()

	if err := w.pipeConnectionToSSHD(ctx, ctrlAddress, ctrlPort, reqParams.Password); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// pipeConnectionToSSHD initiates the connection back to the controller and pipes
// it over to the local SSHD. This call blocks until the connection has finished.
func (w *sshSessionWorker) pipeConnectionToSSHD(ctx context.Context, ctrlAddress, ctrlPort, password string) error {
	controllerConn, err := w.connectionGetter.GetControllerConnection(password, ctrlAddress, ctrlPort)
	if err != nil {
		return errors.Trace(err)
	}
	sshdConn, err := w.connectionGetter.GetSSHDConnection()
	if err != nil {
		return errors.Trace(err)
	}

	// We close the connections when the context is done
	// or, if the connections finish first, we signal the
	// routine to exit with the done channel.
	doneChan := make(chan struct{})
	defer close(doneChan)

	go func() {
		select {
		case <-ctx.Done():
			controllerConn.Close()
			sshdConn.Close()
		case <-doneChan:
		}
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	go func() {
		defer wg.Done()
		// sshd -> conn
		defer controllerConn.Close()
		defer sshdConn.Close()
		_, _ = io.Copy(sshdConn, controllerConn)

	}()

	go func() {
		defer wg.Done()
		// conn -> sshd
		defer controllerConn.Close()
		defer sshdConn.Close()
		_, _ = io.Copy(controllerConn, sshdConn)
	}()
	wg.Wait()
	return nil
}

// ConnectionGetter provides the methods to connect to a controller and the local SSHD.
type ConnectionGetter interface {
	GetControllerConnection(password, ctrlAddress, ctrlPort string) (net.Conn, error)
	GetSSHDConnection() (net.Conn, error)
}

// connectionGetter is capable of initating SSH connections to two places.
//
//  1. The controller's SSH server
//  2. The local SSHD on the machine
//
// The consumer is expect to pipe these connections together.
type connectionGetter struct {
	logger          Logger
	sshdConfigPaths []string
}

// newConnectionGetter returns a new connectionGetter.
func newConnectionGetter(l Logger) *connectionGetter {
	return &connectionGetter{
		logger:          l,
		sshdConfigPaths: []string{"/etc/ssh/sshd_config", "/usr/share/openssh/sshd_config"},
	}
}

// GetControllerConnection initiates an SSH connection to the target ctrlAddress.
func (w *connectionGetter) GetControllerConnection(password, ctrlAddress, port string) (net.Conn, error) {
	// TODO(ale8k): Watch will return host key in subsequent PR.
	sshConfig := &ssh.ClientConfig{
		User:            sshtunneler.ReverseTunnelUser,
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO(ale8k): Fill in host key here
		Timeout:         30 * time.Second,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%s", ctrlAddress, port), sshConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ch, in, err := client.OpenChannel(sshtunneler.JujuTunnelChannel, nil)
	if err != nil {
		return nil, err
	}
	go ssh.DiscardRequests(in)

	return sshconn.NewChannelConn(ch), nil
}

// GetSSHConnection performs a stand TCP dial to the SSHD running on the machine.
func (w *connectionGetter) GetSSHDConnection() (net.Conn, error) {
	port := w.getLocalSSHPort()

	localSSHD, err := net.Dial("tcp", "localhost:"+port)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return localSSHD, nil
}

// getLocalSSHPort parses the local sshd_config file to get the port sshd is listening on.
// It will try all the provided paths.
// If it cannot get it for any reason, it'll log the error and return the default port 22.
func (w *connectionGetter) getLocalSSHPort() string {
	port := "22"

	for _, filePath := range w.sshdConfigPaths {
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
