// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/pki"
)

type AuthorityWorker interface {
	Authority() pki.Authority
	worker.Worker
}

// ManifoldConfig holds the certificate material needed to start the
// certificate-watcher worker. All fields are controller-owned startup
// values supplied at engine-creation time; the manifold does not read
// any config file or look up values through the agent manifold.
type ManifoldConfig struct {
	// CACert is the TLS CA certificate PEM block.
	CACert string

	// CAPrivateKey is the TLS CA private key PEM block. This field
	// is sensitive and must not be logged.
	CAPrivateKey string

	// ControllerCert is the controller TLS certificate PEM block.
	ControllerCert string

	// ControllerPrivateKey is the controller TLS private key PEM block.
	// This field is sensitive and must not be logged.
	ControllerPrivateKey string

	// CertWatcherWorkerFn is an optional override for tests.
	CertWatcherWorkerFn func(config ManifoldConfig) (AuthorityWorker, error)
}

// Validate returns an error if the config is incomplete.
func (c ManifoldConfig) Validate() error {
	if c.CACert == "" {
		return errors.NotValidf("empty CACert")
	}
	if c.CAPrivateKey == "" {
		return errors.NotValidf("empty CAPrivateKey")
	}
	if c.ControllerCert == "" {
		return errors.NotValidf("empty ControllerCert")
	}
	if c.ControllerPrivateKey == "" {
		return errors.NotValidf("empty ControllerPrivateKey")
	}
	return nil
}

// Manifold provides a worker for supplying a pki Authority to other
// workers that want to create and modify certificates in a Juju
// controller.
// The manifold is intended to be a dependency for the apiserver.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			if config.CertWatcherWorkerFn != nil {
				return config.CertWatcherWorkerFn(config)
			}

			w := &apiserverCertWatcher{}
			if err := w.setup(config); err != nil {
				return nil, errors.Annotate(err, "setting up initial ca authority")
			}

			w.tomb.Go(w.loop)
			return w, nil
		},
		Output: outputFunc,
	}
}

func outputFunc(in worker.Worker, out any) error {
	inWorker, _ := in.(AuthorityWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got a %T", inWorker, in)
	}
	switch result := out.(type) {
	case *pki.Authority:
		*result = inWorker.Authority()
	default:
		return errors.Errorf("unexpected type")
	}
	return nil
}

type apiserverCertWatcher struct {
	tomb      tomb.Tomb
	authority pki.Authority
}

func (w *apiserverCertWatcher) loop() error {
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	}
}

func (w *apiserverCertWatcher) Authority() pki.Authority {
	return w.authority
}

// Kill implements worker.Worker.
func (w *apiserverCertWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *apiserverCertWatcher) setup(config ManifoldConfig) error {
	caCert := config.CACert
	if caCert == "" {
		return errors.New("no ca certificate found in config")
	}

	caPrivateKey := config.CAPrivateKey
	if caPrivateKey == "" {
		return errors.New("no CA private key found in config")
	}

	authority, err := pki.NewDefaultAuthorityPemCAKey([]byte(caCert),
		[]byte(caPrivateKey))
	if err != nil {
		return errors.Annotate(err, "building authority from pem ca")
	}

	_, err = authority.LeafGroupFromPemCertKey(pki.DefaultLeafGroup,
		[]byte(config.ControllerCert), []byte(config.ControllerPrivateKey))
	if err != nil {
		return errors.Annotate(err, "loading default certificate for controller")
	}

	_, signers, err := pki.UnmarshalPemData([]byte(config.ControllerPrivateKey))
	if err != nil {
		return errors.Annotate(err, "setting default certificate signing key")
	}
	if len(signers) != 1 {
		return errors.Annotate(err, "expected one signing key from certificate pem data")
	}
	authority.SetLeafSigner(signers[0])

	w.authority = authority
	return nil
}

// Wait implements worker.Worker.
func (w *apiserverCertWatcher) Wait() error {
	return w.tomb.Wait()
}
