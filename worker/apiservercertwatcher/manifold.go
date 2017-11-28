// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher

import (
	"crypto/tls"
	"crypto/x509"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/voyeur"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/dependency"
)

var logger = loggo.GetLogger("juju.worker.apiservercertwatcher")

type ManifoldConfig struct {
	AgentName          string
	AgentConfigChanged *voyeur.Value
}

// Manifold returns a dependency.Manifold which wraps an agent's
// voyeur.Value which is set whenever the agent config is
// changed. The manifold will not bounce when the certificates
// change.
//
// The worker will watch for API server certificate changes,
// and make the current value available via the manifold's Output.
// The Output expects a pointer to a function of type:
//    func() *tls.Certificate
//
// The resulting tls.Certificate's Leaf field will be set, to
// ensure we only parse the certificate once. This allows the
// consumer to obtain the associated DNS names.
//
// The manifold is intended to be a dependency for the apiserver.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if config.AgentConfigChanged == nil {
				return nil, errors.NotValidf("nil AgentConfigChanged")
			}

			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			w := &apiserverCertWatcher{
				agent:              a,
				agentConfigChanged: config.AgentConfigChanged,
			}
			if err := w.update(); err != nil {
				return nil, errors.Annotate(err, "parsing initial certificate")
			}

			go func() {
				defer w.tomb.Done()
				w.tomb.Kill(w.loop())
			}()
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
	outPointer, ok := out.(*func() *tls.Certificate)
	if !ok {
		return errors.Errorf("out should be %T; got %T", outPointer, out)
	}
	*outPointer = inWorker.getCurrent
	return nil
}

type apiserverCertWatcher struct {
	tomb               tomb.Tomb
	agent              agent.Agent
	agentConfigChanged *voyeur.Value

	mu         sync.Mutex
	currentRaw string
	current    *tls.Certificate
}

func (w *apiserverCertWatcher) loop() error {
	watch := w.agentConfigChanged.Watch()
	defer watch.Close()
	done := make(chan struct{})
	defer close(done)

	// TODO(axw) - this is pretty awful. There should be a
	// NotifyWatcher for voyeur.Value. Note also that this code is
	// repeated elsewhere.
	watchCh := make(chan bool)
	go func() {
		defer close(watchCh)
		for watch.Next() {
			select {
			case <-done:
				return
			case watchCh <- true:
			}
		}
	}()

	for {
		// Always unconditionally check for a change first, in case
		// there was a change between the start func and the call
		// to Watch.
		if err := w.update(); err != nil {
			// We don't bounce the worker on bad certificate data.
			logger.Errorf("cannot update certificate: %v", err)
		}
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case _, ok := <-watchCh:
			if !ok {
				return errors.New("config changed value closed")
			}
		}
	}
}

// Kill implements worker.Worker.
func (w *apiserverCertWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *apiserverCertWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *apiserverCertWatcher) update() error {
	//logger.Errorf("cannot update certificate: %v", err)
	config := w.agent.CurrentConfig()
	info, ok := config.StateServingInfo()
	if !ok {
		return errors.New("no state serving info in agent config")
	}
	if info.Cert == "" {
		return errors.New("certificate is empty")
	}
	if info.PrivateKey == "" {
		return errors.New("private key is empty")
	}
	if info.Cert == w.currentRaw {
		// No change.
		return nil
	}

	tlsCert, err := tls.X509KeyPair([]byte(info.Cert), []byte(info.PrivateKey))
	if err != nil {
		return errors.Annotatef(err, "cannot create new TLS certificate")
	}
	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return errors.Annotatef(err, "parsing x509 cert")
	}
	tlsCert.Leaf = x509Cert

	w.currentRaw = info.Cert
	w.mu.Lock()
	w.current = &tlsCert
	w.mu.Unlock()

	var addr []string
	for _, ip := range x509Cert.IPAddresses {
		addr = append(addr, ip.String())
	}
	logger.Infof("new certificate addresses: %v", strings.Join(addr, ", "))
	return nil
}

func (w *apiserverCertWatcher) getCurrent() *tls.Certificate {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.current
}
