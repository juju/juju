// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorefacade

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/worker/fortress"
)

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	ObjectStoreName string
	FortressName    string

	NewWorker func(Config) (worker.Worker, error)
	Logger    logger.Logger
}

// validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if config.FortressName == "" {
		return errors.NotValidf("empty FortressName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ObjectStoreName,
			config.FortressName,
		},
		Start:  config.start,
		Output: config.output,
	}
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var objectStoreGetter coreobjectstore.ObjectStoreGetter
	if err := getter.Get(config.ObjectStoreName, &objectStoreGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var guest fortress.Guest
	if err := getter.Get(config.FortressName, &guest); err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		FortressVisitor:   guest,
		ObjectStoreGetter: objectStoreGetter,
		Logger:            config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

func (config ManifoldConfig) output(in worker.Worker, out any) error {
	w, ok := in.(*Worker)
	if !ok {
		return errors.Errorf("expected input of Worker, got %T", in)
	}

	switch out := out.(type) {
	case *coreobjectstore.ObjectStoreGetter:
		var target coreobjectstore.ObjectStoreGetter = w
		*out = target
	default:
		return errors.Errorf("expected output of ObjectStoreGetter, got %T", out)
	}
	return nil
}
