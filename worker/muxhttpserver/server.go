// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package muxhttpserver

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pki"
)

type Config struct {
	Address string
	Port    string
}

type Logger interface {
	Errorf(string, ...interface{})
	Infof(string, ...interface{})
}

type Server struct {
	catacomb catacomb.Catacomb
	listener net.Listener
	logger   Logger
	Mux      *apiserverhttp.Mux
	server   *http.Server
}

var (
	defaultPort = "17071"
)

func NewServer(authority pki.Authority, logger Logger, conf Config) (*Server, error) {
	mux := apiserverhttp.NewMux()

	tlsConfig := &tls.Config{
		GetCertificate: pki.AuthoritySNITLSGetter(authority),
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%s", conf.Address, conf.Port))
	if err != nil {
		return nil, errors.Annotate(err, "creating mux http server listener")
	}

	httpServ := &http.Server{
		Handler:   mux,
		TLSConfig: tlsConfig,
	}

	server := &Server{
		listener: listener,
		logger:   logger,
		Mux:      mux,
		server:   httpServ,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &server.catacomb,
		Work: server.loop,
	}); err != nil {
		return server, errors.Trace(err)
	}
	return server, nil
}

func DefaultConfig() Config {
	return Config{
		Port: defaultPort,
	}
}

// Kill implements the worker interface
func (s *Server) Kill() {
	s.catacomb.Kill(nil)
}

func (s *Server) Port() string {
	splits := strings.Split(s.listener.Addr().String(), ":")
	if len(splits) == 0 {
		return ""
	}
	return splits[len(splits)-1]
}

// Wait implements the worker interface
func (s *Server) Wait() error {
	return s.catacomb.Wait()
}

func (s *Server) loop() error {
	httpCh := make(chan error)

	go func() {
		s.logger.Infof("starting http server on %s", s.listener.Addr())
		httpCh <- s.server.ServeTLS(s.listener, "", "")
		close(httpCh)
	}()

	for {
		select {
		case <-s.catacomb.Dying():
			s.server.Close()
		case err := <-httpCh:
			if err != nil && err != http.ErrServerClosed {
				return err
			}
			return s.catacomb.ErrDying()
		}
	}
}
