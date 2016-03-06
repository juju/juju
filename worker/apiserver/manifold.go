// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package apiserver

import (
	"github.com/juju/errors"
	"github.com/juju/replicaset"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	workerstate "github.com/juju/juju/worker/state"
)

// ManifoldConfig defines the apiserver's dependencies.
type ManifoldConfig struct {
	AgentName          string
	StateName          string
	NewApiserverWorker func(st *state.State, certChanged chan params.StateServingInfo) (worker.Worker, error)
}

// Manifold creates a manifold that runs a apiserver worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	certChangedChan := make(chan params.StateServingInfo, 10)
	return dependency.Manifold{
		Output: outputFunc,
		Inputs: []string{config.AgentName, config.StateName},
		Start: func(getResource dependency.GetResourceFunc) (worker.Worker, error) {
			var a agent.Agent
			if err := getResource(config.AgentName, &a); err != nil {
				return nil, errors.Trace(err)
			}

			var stTracker workerstate.StateTracker
			if err := getResource(config.StateName, &stTracker); err != nil {
				return nil, err
			}

			st, err := stTracker.Use()
			if err != nil {
				return nil, errors.Annotate(err, "acquiring state")
			}
			w, err := NewWorker(st, config.NewApiserverWorker, certChangedChan)
			if err != nil {
				return nil, errors.Annotate(err, "cannot start apiserver worker")
			}

			// When the state workers are done, indicate that we no
			// longer need the State.
			go func() {
				w.Wait()
				stTracker.Done()
			}()
			return w, nil
		},
	}
}

// Variable to override in tests, default is true
var ProductionMongoWriteConcern = true

func dialOpts() mongo.DialOpts {
	dOpts := mongo.DefaultDialOpts()
	dOpts.PostDial = func(session *mgo.Session) error {
		safe := mgo.Safe{}
		if ProductionMongoWriteConcern {
			safe.J = true
			_, err := replicaset.CurrentConfig(session)
			if err == nil {
				// set mongo to write-majority (writes only returned after
				// replicated to a majority of replica-set members).
				safe.WMode = "majority"
			}
		}
		session.SetSafe(&safe)
		return nil
	}
	return dOpts
}

// outputFunc extracts the CertChanger from an *apiServerWorker passed in as a Worker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*apiServerWorker)
	if inWorker == nil {
		return errors.Errorf("expected *apiServerWorker input; got %T", in)
	}
	outPointer, _ := out.(*CertChanger)
	if outPointer == nil {
		return errors.Errorf("expected *apiserver.CertChanger output; got %T", out)
	}
	*outPointer = inWorker
	return nil
}
