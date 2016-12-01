// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"io"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/worker"
)

func NewRemoteRelationsFacade(apiCaller base.APICaller) (RemoteRelationsFacade, error) {
	facade := remoterelations.NewClient(apiCaller)
	return facade, nil
}

func NewWorker(config Config) (worker.Worker, error) {
	w, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func apiConnForModelFunc(
	a agent.Agent,
	apiOpen func(*api.Info, api.DialOpts) (api.Connection, error),
) (func(string) (api.Connection, error), error) {
	agentConf := a.CurrentConfig()
	apiInfo, ok := agentConf.APIInfo()
	if !ok {
		return nil, errors.New("no API connection details")
	}
	return func(modelUUID string) (api.Connection, error) {
		apiInfo.ModelTag = names.NewModelTag(modelUUID)
		conn, err := apiOpen(apiInfo, api.DialOpts{
			Timeout:    time.Second,
			RetryDelay: 200 * time.Millisecond,
		})
		if err != nil {
			return nil, errors.Annotate(err, "failed to open API to remote model")
		}
		return conn, nil
	}, nil
}

// relationChangePublisherForModelFunc returns a function that
// can be used be construct instances which publish remote relation
// changes for a given model.

// For now we use a facade on the same controller, but in future this
// may evolve into a REST caller.
func relationChangePublisherForModelFunc(
	apiConnForModelFunc func(string) (api.Connection, error),
) func(string) (RemoteRelationChangePublisherCloser, error) {
	return func(modelUUID string) (RemoteRelationChangePublisherCloser, error) {
		conn, err := apiConnForModelFunc(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		facade, err := NewRemoteRelationsFacade(conn)
		if err != nil {
			conn.Close()
			return nil, errors.Trace(err)
		}
		return &publisherCloser{facade, conn}, nil
	}
}

type publisherCloser struct {
	RemoteRelationChangePublisher
	conn io.Closer
}

func (p *publisherCloser) Close() error {
	return p.Close()
}
