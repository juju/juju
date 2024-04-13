// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasrbacmapper

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"k8s.io/client-go/informers"

	"github.com/juju/juju/internal/provider/caas"
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

func (c ManifoldConfig) Start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var broker caas.Broker
	if err := getter.Get(c.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}
	k8sBroker, ok := broker.(K8sBroker)
	if !ok {
		return nil, errors.Errorf("broker does not implement K8sBroker")
	}

	return NewMapper(c.Logger, k8sBroker.SharedInformerFactory().Core().V1().ServiceAccounts())
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
