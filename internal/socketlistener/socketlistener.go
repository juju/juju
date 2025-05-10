// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package socketlistener

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/juju/sockets"
)

// Config represents configuration for the socketlistener worker.
type Config struct {
	Logger logger.Logger
	// SocketName is the socket file descriptor.
	SocketName string
	// RegisterHandlers should register handlers on the router with
	// router.HandlerFunc or similar.
	RegisterHandlers func(router *mux.Router)
	// ShutdownTimeout is how long the socketlistener has to gracefully shutdown
	// when Kill is called on the worker.
	ShutdownTimeout time.Duration
}

// Validate returns an error if config cannot drive the SocketListener.
func (config Config) Validate() error {
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if config.RegisterHandlers == nil {
		return errors.NotValidf("nil RegisterHandlers func")
	}
	if config.ShutdownTimeout == 0 {
		return errors.NotValidf("zero value for ShutdownTimeout")
	}
	return nil
}

// SocketListener is a socketlistener worker.
type SocketListener struct {
	config   Config
	tomb     tomb.Tomb
	listener net.Listener
}

// NewSocketListener returns a socketlistener with the given config.
func NewSocketListener(config Config) (*SocketListener, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	l, err := sockets.Listen(sockets.Socket{
		Address: config.SocketName,
		Network: "unix",
	})
	if err != nil {
		return nil, errors.Annotate(err, "unable to listen on unix socket")
	}
	config.Logger.Debugf(context.TODO(), "socketlistener listening on socket %q", config.SocketName)

	sl := &SocketListener{
		config:   config,
		listener: l,
	}
	sl.tomb.Go(sl.run)
	return sl, nil
}

// Kill is part of the Worker.worker interface.
func (sl *SocketListener) Kill() {
	sl.tomb.Kill(nil)
}

// Wait is part of the Worker.worker interface.
func (sl *SocketListener) Wait() error {
	return sl.tomb.Wait()
}

// run listens on the control socket and handles requests.
func (sl *SocketListener) run() error {
	ctx, cancel := sl.scopedContext()
	defer cancel()

	router := mux.NewRouter()
	sl.config.RegisterHandlers(router)

	srv := http.Server{Handler: router}
	defer func() {
		err := srv.Close()
		if err != nil {
			sl.config.Logger.Warningf(ctx, "error closing HTTP server: %v", err)
		}
	}()

	sl.tomb.Go(func() error {
		// Wait for the tomb to start dying and then shut the server down.
		<-sl.tomb.Dying()
		ctx, cancel := context.WithTimeout(context.Background(), sl.config.ShutdownTimeout)
		defer cancel()
		return srv.Shutdown(ctx)
	})

	sl.config.Logger.Debugf(ctx, "socketlistener now serving on socket %q", sl.config.SocketName)
	defer sl.config.Logger.Debugf(ctx, "socketlistener finished serving on socket %q", sl.config.SocketName)
	if err := srv.Serve(sl.listener); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (sl *SocketListener) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(sl.tomb.Context(context.Background()))
}
