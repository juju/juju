// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiservercertwatcher

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/internal/pki"
)

// OperatorWatcher is responsible for creating a new PKI certificate chain to
// use in operators that need to start their own HTTPS servers.
// TODO this watcher should be replaced in the future to use an intermediate CA
// from the controller
type PKIAuthorityWorker struct {
	authority pki.Authority
	catacomb  catacomb.Catacomb
}

func NewAuthorityWorker(_ agent.Agent) (AuthorityWorker, error) {
	return newPKIAuthorityWorker()
}

func newPKIAuthorityWorker() (*PKIAuthorityWorker, error) {
	signer, err := pki.DefaultKeyProfile()
	if err != nil {
		return nil, errors.Annotate(err, "creating agent watcher signer")
	}

	cert, err := pki.NewCA("juju agent", signer)
	if err != nil {
		return nil, errors.Annotate(err, "creating agent ca certificate")
	}

	authority, err := pki.NewDefaultAuthority(cert, signer)
	if err != nil {
		return nil, errors.Annotate(err, "creating authority for agent ca and signer")
	}

	agentWatcher := &PKIAuthorityWorker{
		authority: authority,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "pki-authority",
		Site: &agentWatcher.catacomb,
		Work: agentWatcher.loop,
	}); err != nil {
		return agentWatcher, errors.Trace(err)
	}
	return agentWatcher, nil
}

func (a *PKIAuthorityWorker) Authority() pki.Authority {
	return a.authority
}

func (a *PKIAuthorityWorker) Kill() {
	a.catacomb.Kill(nil)
}

func (a *PKIAuthorityWorker) Wait() error {
	return a.catacomb.Wait()
}

func (a *PKIAuthorityWorker) loop() error {
	select {
	case <-a.catacomb.Dying():
		return a.catacomb.ErrDying()
	}
}
