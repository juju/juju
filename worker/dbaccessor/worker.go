// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/utils"

	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
)

const dqlitePort = 17666

type DBGetter interface {
	GetDB(modelUUID string) (*sql.DB, error)
	GetExistingDB(modelUUID string) (*sql.DB, error)
}

// Hub provides an API for publishing and subscribing to incoming messages.
type Hub interface {
	Publish(topic string, data interface{}) (func(), error)
	Subscribe(topic string, handler interface{}) (func(), error)
}

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	ControllerCACert  []byte
	ControllerCert    []byte
	ControllerCertKey []byte
	APIAddrs          []string
	DataDir           string
	Clock             clock.Clock
	Logger            Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if len(c.ControllerCACert) == 0 {
		return errors.NotValidf("missing controller CA certificate")
	}
	if len(c.ControllerCert) == 0 {
		return errors.NotValidf("missing controller certificate")
	}
	if len(c.APIAddrs) == 0 {
		return errors.NotValidf("missing API addresses")
	}
	if c.DataDir == "" {
		return errors.NotValidf("missing data dir")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing logger")
	}
	return nil
}

type DBApp interface {
	// Open the dqlite database with the given name
	Open(context.Context, string) (*sql.DB, error)
	// Ready can be used to wait for a node to complete some initial tasks that are
	// initiated at startup. For example a brand new node will attempt to join the
	// cluster, a restarted node will check if it should assume some particular
	// role, etc.
	//
	// If this method returns without error it means that those initial tasks have
	// succeeded and follow-up operations like Open() are more likely to succeeed
	// quickly.
	Ready(context.Context) error
	// Handover transfers all responsibilities for this node (such has leadership
	// and voting rights) to another node, if one is available.
	//
	// This method should always be called before invoking Close(), in order to
	// gracefully shutdown a node.
	Handover(context.Context) error
	// ID returns the dqlite ID of this application node.
	ID() uint64
	// Close the application node, releasing all resources it created.
	Close() error
}

type REPL interface {
	worker.Worker
}

type dbWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	mu        sync.RWMutex
	dbApp     DBApp
	dbReadyCh chan struct{}
	dbHandles map[string]*sql.DB
}

func NewWorker(cfg WorkerConfig) (*dbWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &dbWorker{
		cfg:       cfg,
		dbHandles: make(map[string]*sql.DB),
		dbReadyCh: make(chan struct{}),
	}

	if err = catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *dbWorker) loop() (err error) {
	defer func() {
		w.mu.Lock()
		if w.dbApp != nil {
			if hErr := w.dbApp.Handover(context.TODO()); hErr != nil {
				if err == nil {
					err = errors.Annotate(err, "gracefully handing over dqlite node responsibilities")
				} else { // we are exiting with another error; so we just log this.
					w.cfg.Logger.Errorf("unable to gracefully hand off dqlite node responsibilities: %v", hErr)
				}
			}

			if cErr := w.dbApp.Close(); cErr != nil {
				if err == nil {
					err = errors.Annotate(cErr, "closing dqlite application instance")
				} else { // we are exiting with another error; so we just log this.
					w.cfg.Logger.Errorf("unable to close dqlite application instance: %v", cErr)
				}
			}
			w.dbApp = nil
		}
		w.mu.Unlock()
	}()

	if err := w.initializeDqlite(); err != nil {
		return errors.Trace(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		}
	}
}

// Kill is part of the worker.Worker interface.
func (w *dbWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *dbWorker) Wait() error {
	return w.catacomb.Wait()
}

// GetDB returns a sql.DB handle for the dqlite-backed database that contains
// the data for the specified model UUID.
func (w *dbWorker) GetDB(modelUUID string) (*sql.DB, error) {
	select {
	case <-w.dbReadyCh:
		// dqlite has been initialized
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if a DB handle has already been created and cached.
	if cachedDB := w.dbHandles[modelUUID]; cachedDB != nil {
		return cachedDB, nil
	}

	// Create and cache DB instance.
	db, err := w.dbApp.Open(context.TODO(), modelUUID)
	if err != nil {
		return nil, errors.Annotatef(err, "acccessing DB for model %q", modelUUID)
	}
	if err := ensureDBSchema(modelUUID, db); err != nil {
		return nil, errors.Annotatef(err, "acccessing DB for model %q", modelUUID)
	}
	w.dbHandles[modelUUID] = db

	return db, nil
}

// GetExistingDB returns a sql.DB handle for the dqlite-backed database that
// contains the data for the specified model UUID or a NotFound error if the
// DB is not known.
func (w *dbWorker) GetExistingDB(modelUUID string) (*sql.DB, error) {
	select {
	case <-w.dbReadyCh:
		// dqlite has been initialized
	case <-w.catacomb.Dying():
		return nil, w.catacomb.ErrDying()
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if a DB handle has already been created and cached.
	if cachedDB := w.dbHandles[modelUUID]; cachedDB != nil {
		return cachedDB, nil
	}

	return nil, errors.NotFoundf("database for model %q", modelUUID)
}

func (w *dbWorker) initializeDqlite() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Dqlite has already been initialized
	if w.dbApp != nil {
		return nil
	}

	// Use the controller CA/cert to setup TLS for the dqlite peers.
	dqServerTLSConf, dqClientTLSConf, err := w.getDQliteTLSConfiguration()
	if err != nil {
		return errors.Annotatef(err, "configuring TLS support for dqlite cluster")
	}

	// Identify a cloud-local address on this machine that we can advertise
	// to other peers.
	localAddr, err := detectLocalDQliteAddress()
	if err != nil {
		return errors.Annotatef(err, "detecting local dqlite address")
	}

	// Scan the API address list and identify any suitable dqlite peer
	// addresses that dqlite should attempt to connect to.
	peerAddrs, err := detectDQlitePeerAddresses(w.cfg.APIAddrs)
	if err != nil {
		return errors.Annotatef(err, "detecting local dqlite address")
	}

	appOpts := []dqApp.Option{
		dqApp.WithAddress(localAddr),
		dqApp.WithCluster(peerAddrs),
		dqApp.WithTLS(dqServerTLSConf, dqClientTLSConf),
	}

	// Start dqlite
	if err := os.MkdirAll(w.cfg.DataDir, 0700); err != nil {
		return errors.Annotatef(err, "creating directory for dqlite data")
	}

	w.cfg.Logger.Infof("initializing dqlite application with local address %q and peer addresses %v", localAddr, peerAddrs)
	if w.dbApp, err = NewApp(w.cfg.DataDir, appOpts...); err != nil {
		return errors.Trace(err)
	}

	// Ensure dqlite is ready to acccept new changes
	if err := w.dbApp.Ready(context.TODO()); err != nil {
		_ = w.dbApp.Close()
		w.dbApp = nil
		return errors.Annotatef(err, "ensuring dqlite is ready to process changes")
	}

	// Start REPL
	replSocket := filepath.Join(w.cfg.DataDir, "juju.sock")
	_ = os.Remove(replSocket)
	repl, err := newREPL(replSocket, w, w.cfg.Clock, w.cfg.Logger)
	if err != nil {
		_ = w.dbApp.Close()
		w.dbApp = nil
		return errors.Annotatef(err, "starting dqlite REPL")
	}
	w.catacomb.Add(repl)

	w.cfg.Logger.Infof("initialized dqlite application (ID: %v)", w.dbApp.ID())
	close(w.dbReadyCh) // start accepting GetDB() requests.
	return nil
}

func (w *dbWorker) getDQliteTLSConfiguration() (serverConf, clientConf *tls.Config, err error) {
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(w.cfg.ControllerCACert)

	controllerCert, err := tls.X509KeyPair(w.cfg.ControllerCert, w.cfg.ControllerCertKey)
	if err != nil {
		return nil, nil, errors.Annotate(err, "parsing controller certificate")
	}

	return &tls.Config{
			ClientCAs:    caCertPool,
			Certificates: []tls.Certificate{controllerCert},
		},
		&tls.Config{
			RootCAs:      caCertPool,
			Certificates: []tls.Certificate{controllerCert},
			// We cannot provide a ServerName value here so instead we
			// rely on the server validating the controller's client cert.
			InsecureSkipVerify: true,
		},
		nil
}

func detectLocalDQliteAddress() (string, error) {
	sysAddrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", errors.Annotate(err, "querying addresses of system NICs")
	}

	// DQlite nodes should only advertise cloud-local addresses to their
	// peers. To figure out the address to bind to, we need to scan all NIC
	// addresses and return the first one that has cloud-local scope.
	for _, sysAddr := range sysAddrs {
		var host string

		switch v := sysAddr.(type) {
		case *net.IPNet:
			host = v.IP.String()
		case *net.IPAddr:
			host = v.IP.String()
		}

		if host == "" {
			continue
		}

		machAddr := network.NewMachineAddress(host)
		if machAddr.Scope == network.ScopeCloudLocal {
			return fmt.Sprintf("%s:%d", machAddr.Value, dqlitePort), nil
		}
	}

	return "", errors.NewNotFound(nil, "unable to find suitable local address for advertising to dqlite peers")
}

func detectDQlitePeerAddresses(apiAddrs []string) ([]string, error) {
	if len(apiAddrs) == 0 {
		return nil, errors.Errorf("no available API addresses")
	}

	// Since the address list may contain VIPs (i.e. IPs not visible to the
	// machine) or FAN addresses, we will create a TCP listener on an
	// unused port (all addresses) that servers a random nonce. We will
	// then try to connect to that port on each provided dqlite address
	// until we reach the server and read back the nonce.
	nonceUUID, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	nonce := nonceUUID.String()

	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return nil, errors.Annotatef(err, "creating TCP listener for detecting dqlite peer addresses")
	}
	defer func() { _ = l.Close() }()
	go serveNonce(l, nonce)

	// Try to read the nonce from each of the provided addresses in parallel
	var (
		wg         sync.WaitGroup
		noncePort  = l.Addr().(*net.TCPAddr).Port
		peerAddrCh = make(chan string, len(apiAddrs))
	)

	for _, addr := range apiAddrs {
		// We should only connect on cloud-local addresses
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, errors.Trace(err)
		}
		machAddr := network.NewMachineAddress(host)
		if machAddr.Scope != network.ScopeCloudLocal {
			continue
		}

		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			nonceAddr := fmt.Sprintf("%s:%d", host, noncePort)
			if !maybeReadNonce(nonceAddr, nonce) {
				peerAddrCh <- fmt.Sprintf("%s:%d", host, dqlitePort)
			}
		}(addr)
	}
	wg.Wait()

	// Collect discovered peer addresses.
	close(peerAddrCh)
	var peerAddrs []string
	for addr := range peerAddrCh {
		peerAddrs = append(peerAddrs, addr)
	}
	return peerAddrs, nil
}

func serveNonce(l net.Listener, nonce string) {
	for {
		s, err := l.Accept()
		if err != nil {
			return
		}

		_, _ = s.Write([]byte(nonce))
		_ = s.Close()
	}
}

func maybeReadNonce(nonceAddr string, expNonce string) bool {
	conn, err := net.DialTimeout("tcp", nonceAddr, time.Second)
	if err != nil {
		return false // assume non-reachable
	}
	defer func() { _ = conn.Close() }()

	nonceBuf := make([]byte, len(expNonce))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := conn.Read(nonceBuf)

	return n == len(expNonce) && string(nonceBuf[:n]) == expNonce
}
