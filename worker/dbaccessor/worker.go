// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"context"
	"database/sql"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dqApp "github.com/canonical/go-dqlite/app"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/worker/v3/catacomb"
)

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
	DQLiteAddrs []string
	DataDir     string
	Clock       clock.Clock
	Logger      Logger
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if len(c.DQLiteAddrs) == 0 {
		return errors.NotValidf("missing dqlite cluster addresses")
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

type dbWorker struct {
	cfg      WorkerConfig
	catacomb catacomb.Catacomb

	mu            sync.RWMutex
	dqliteApp     *dqApp.App
	dqliteReadyCh chan struct{}
	dbHandles     map[string]*sql.DB
}

func NewWorker(cfg WorkerConfig) (*dbWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &dbWorker{
		cfg:           cfg,
		dbHandles:     make(map[string]*sql.DB),
		dqliteReadyCh: make(chan struct{}),
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
		if w.dqliteApp != nil {
			if hErr := w.dqliteApp.Handover(context.TODO()); hErr != nil {
				if err == nil {
					err = errors.Annotate(err, "gracefully handing over dqlite node responsibilities")
				} else { // we are exiting with another error; so we just log this.
					w.cfg.Logger.Errorf("unable to gracefully hand off dqlite node responsibilities: %v", hErr)
				}
			}

			if cErr := w.dqliteApp.Close(); cErr != nil {
				if err == nil {
					err = errors.Annotate(cErr, "closing dqlite application instance")
				} else { // we are exiting with another error; so we just log this.
					w.cfg.Logger.Errorf("unable to close dqlite application instance: %v", cErr)
				}
			}
			w.dqliteApp = nil
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
	case <-w.dqliteReadyCh:
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
	db, err := w.dqliteApp.Open(context.TODO(), modelUUID)
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
	case <-w.dqliteReadyCh:
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
	if w.dqliteApp != nil {
		return nil
	}

	localAddr, peerAddrs, err := detectLocalDQliteAddr(w.cfg.DQLiteAddrs)
	if err != nil {
		return errors.Annotatef(err, "detecting local dqlite address")
	}

	if err := os.MkdirAll(w.cfg.DataDir, 0700); err != nil {
		return errors.Annotatef(err, "creating directory for dqlite data")
	}

	w.cfg.Logger.Infof("initializing dqlite application with local address %q and peer addresses %v", localAddr, peerAddrs)
	if w.dqliteApp, err = dqApp.New(w.cfg.DataDir, dqApp.WithAddress(localAddr), dqApp.WithCluster(peerAddrs)); err != nil {
		return errors.Trace(err)
	}

	// Ensure dqlite is ready to acccept new changes
	if err := w.dqliteApp.Ready(context.TODO()); err != nil {
		_ = w.dqliteApp.Close()
		w.dqliteApp = nil
		return errors.Annotatef(err, "ensuring dqlite is ready to process changes")
	}

	// Start REPL
	replSocket := filepath.Join(w.cfg.DataDir, "juju.sock")
	_ = os.Remove(replSocket)
	repl, err := newREPL(replSocket, w, w.cfg.Clock, w.cfg.Logger)
	if err != nil {
		_ = w.dqliteApp.Close()
		w.dqliteApp = nil
		return errors.Annotatef(err, "starting dqlite REPL")
	}
	w.catacomb.Add(repl)

	w.cfg.Logger.Infof("initialized dqlite application (ID: %v)", w.dqliteApp.ID())
	close(w.dqliteReadyCh) // start accepting GetDB() requests.
	return nil
}

func detectLocalDQliteAddr(dqliteAddrs []string) (localAddr string, peerAddrs []string, err error) {
	if len(dqliteAddrs) == 0 {
		return "", nil, errors.Errorf("no available dqlite addresses")
	}

	// The dqlite address list is generated and validated by the manifold
	// code so we can safely assume that they include a port section.
	listenAddr := dqliteAddrs[0][strings.IndexRune(dqliteAddrs[0], ':'):]

	// Since the address list may contain VIPs (i.e. IPs not visible to the
	// machine) or FAN addresses, we will create a TCP socket that listens
	// on the dqlite port on all addresses and try connecting to each
	// provided dqliteAddrs until we reach the server.
	nonceUUID, err := utils.NewUUID()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	nonce := nonceUUID.String()

	l, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", nil, errors.Annotatef(err, "creating TCP listener for detecting local dqlite address")
	}
	defer func() { _ = l.Close() }()
	go serveNonce(l, nonce)

	// Try to read the nonce from each of the provided addresses in parallel
	var wg sync.WaitGroup
	wg.Add(len(dqliteAddrs))
	for _, addr := range dqliteAddrs {
		go func(addr string) {
			defer wg.Done()
			if maybeReadNonce(addr, nonce) {
				localAddr = addr
			}
		}(addr)
	}
	wg.Wait()

	if localAddr == "" {
		return "", nil, errors.Annotatef(err, "unable to detect local dqlite address")
	}

	for _, addr := range dqliteAddrs {
		if addr == localAddr {
			continue
		}

		peerAddrs = append(peerAddrs, addr)
	}

	return localAddr, peerAddrs, nil
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

func maybeReadNonce(addr string, expNonce string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false // assume non-reachable
	}
	defer func() { _ = conn.Close() }()

	nonceBuf := make([]byte, len(expNonce))
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _ := conn.Read(nonceBuf)

	return n == len(expNonce) && string(nonceBuf[:n]) == expNonce
}
