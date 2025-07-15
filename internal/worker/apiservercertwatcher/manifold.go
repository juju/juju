// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/pki"
)

type AuthorityWorker interface {
	Authority() pki.Authority
	worker.Worker
}

type NewCertWatcherWorker func(agent.Agent) (AuthorityWorker, error)

type ManifoldConfig struct {
	AgentName           string
	CertWatcherWorkerFn NewCertWatcherWorker
}

// The manifold is intended to be a dependency for the apiserver.
// Manifold provides a worker for supplying a pki Authority to other workers
// that want to create and modify certificates in a Juju controller.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			if config.CertWatcherWorkerFn != nil {
				return config.CertWatcherWorkerFn(a)
			}

			w := &apiserverCertWatcher{
				agent: a,
			}
			if err := w.setup(); err != nil {
				return nil, errors.Annotate(err, "setting up initial ca authority")
			}

			w.tomb.Go(w.loop)
			return w, nil
		},
		Output: outputFunc,
	}
}

func outputFunc(in worker.Worker, out interface{}) error {
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
	agent     agent.Agent
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

func (w *apiserverCertWatcher) setup() error {
	config := w.agent.CurrentConfig()
	info, ok := config.ControllerAgentInfo()
	if !ok {
		return errors.New("no controller agent info in agent config")
	}

	caCert := config.CACert()
	if caCert == "" {
		return errors.New("no ca certificate found in config")
	}

	caPrivateKey := info.CAPrivateKey
	if caPrivateKey == "" {
		return errors.New("no CA cert private key")
	}

	authority, err := pki.NewDefaultAuthorityPemCAKey([]byte(caCert),
		[]byte(caPrivateKey))
	if err != nil {
		return errors.Annotate(err, "building authority from pem ca")
	}

	_, err = authority.LeafGroupFromPemCertKey(pki.DefaultLeafGroup,
		[]byte(info.Cert), []byte(info.PrivateKey))
	if err != nil {
		return errors.Annotate(err, "loading default certificate for controller")
	}

	_, signers, err := pki.UnmarshalPemData([]byte(info.PrivateKey))
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
