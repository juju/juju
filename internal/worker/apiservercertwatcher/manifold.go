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

// CertMaterial holds the certificate material needed to construct the
// controller authority.
type CertMaterial struct {
	// CACert is the TLS CA certificate PEM block for the controller.
	CACert string

	// CAPrivateKey is the TLS CA private key PEM block.
	CAPrivateKey string

	// ControllerCert is the controller TLS certificate PEM block.
	ControllerCert string

	// ControllerPrivateKey is the controller TLS private key PEM
	// block.
	ControllerPrivateKey string
}

// CertReader returns the current controller certificate material when the
// manifold starts.
type CertReader interface {
	CertMaterial() (CertMaterial, error)
}

// ManifoldConfig holds the certificate material needed to start the
// certificate-watcher worker. The reader is evaluated when the manifold starts
// so bounced workers observe current controller certificate material.
type ManifoldConfig struct {
	// CertReader returns the current controller certificate material.
	CertReader CertReader

	// CertWatcherWorkerFn is an optional override for tests.
	CertWatcherWorkerFn func(material CertMaterial) (AuthorityWorker, error)
}

// Validate returns an error if the config is incomplete.
func (c ManifoldConfig) Validate() error {
	if c.CertReader == nil {
		return errors.NotValidf("nil CertReader")
	}
	return nil
}

// Validate returns an error if the cert material is incomplete.
func (m CertMaterial) Validate() error {
	if m.CACert == "" {
		return errors.NotValidf("empty CACert")
	}
	if m.CAPrivateKey == "" {
		return errors.NotValidf("empty CAPrivateKey")
	}
	if m.ControllerCert == "" {
		return errors.NotValidf("empty ControllerCert")
	}
	if m.ControllerPrivateKey == "" {
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

			material, err := config.CertReader.CertMaterial()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if err := material.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			if config.CertWatcherWorkerFn != nil {
				return config.CertWatcherWorkerFn(material)
			}

			w := &apiserverCertWatcher{}
			if err := w.setup(material); err != nil {
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

func (w *apiserverCertWatcher) setup(material CertMaterial) error {
	caCert := material.CACert
	if caCert == "" {
		return errors.New("no ca certificate found in config")
	}

	caPrivateKey := material.CAPrivateKey
	if caPrivateKey == "" {
		return errors.New("no CA private key found in config")
	}

	authority, err := pki.NewDefaultAuthorityPemCAKey([]byte(caCert),
		[]byte(caPrivateKey))
	if err != nil {
		return errors.Annotate(err, "building authority from pem ca")
	}

	_, err = authority.LeafGroupFromPemCertKey(pki.DefaultLeafGroup,
		[]byte(material.ControllerCert), []byte(material.ControllerPrivateKey))
	if err != nil {
		return errors.Annotate(err, "loading default certificate for controller")
	}

	_, signers, err := pki.UnmarshalPemData([]byte(material.ControllerPrivateKey))
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
