// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasrbacmapper

import (
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	"k8s.io/client-go/informers"
)

type K8sBroker interface {
	SharedInformerFactory() informers.SharedInformerFactory
}

type Logger interface {
	Errorf(string, ...interface{})
}

type ManifoldConfig struct {
	BrokerName string
	Logger     Logger
}

func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.BrokerName,
		},
		Output: manifoldOutput,
		Start:  config.Start,
	}
}

func manifoldOutput(in worker.Worker, out interface{}) error {
	inMapper, ok := in.(Mapper)
	if !ok {
		return errors.Errorf("expected Mapper, got %T", in)
	}

	switch result := out.(type) {
	case *Mapper:
		*result = inMapper
	default:
		return errors.Errorf("expected Mapper, got %T", out)
	}
	return nil
}

func (c ManifoldConfig) Start(context dependency.Context) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var broker K8sBroker
	if err := context.Get(c.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	return NewMapper(c.Logger, broker.SharedInformerFactory().Core().V1().ServiceAccounts())
}

func (c ManifoldConfig) Validate() error {
	if c.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}
