// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/pki"
)

var logger = loggo.GetLogger("juju.worker.apiservercertwatcher")

type ManifoldConfig struct {
	AgentName string
}

// The manifold is intended to be a dependency for the apiserver.
// Manifold provides a worker for supplying a pki Authority to other workers
// that want to create and modify certificates in a Juju controller.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(context dependency.Context) (worker.Worker, error) {
			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
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
	inWorker, _ := in.(*apiserverCertWatcher)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got a %T", inWorker, in)
	}
	switch result := out.(type) {
	case *pki.Authority:
		*result = inWorker.authority
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

// Kill implements worker.Worker.
func (w *apiserverCertWatcher) Kill() {
	w.tomb.Kill(nil)
}

func (w *apiserverCertWatcher) setup() error {
	config := w.agent.CurrentConfig()
	info, ok := config.StateServingInfo()
	if !ok {
		return errors.New("no state serving info in agent config")
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
