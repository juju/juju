// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/worker/dependency"
)

const (
	engineErrorDelay  = 3 * time.Second
	engineBounceDelay = 10 * time.Second
)

func newEngine() (dependency.Engine, error) {
	config := newEngineConfig()

	engine, err := dependency.NewEngine(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return engine, nil
}

func newEngineConfig() dependency.EngineConfig {
	return dependency.EngineConfig{
		IsFatal:       isFatal,
		MoreImportant: moreImportant,
		ErrorDelay:    engineErrorDelay,
		BounceDelay:   engineBounceDelay,
	}
}

// isFatal is an implementation of the IsFatal function in
// dependency.EnginConfig.
func isFatal(err error) bool {
	return false
}

// moreImportant is an implementation of the MoreImportant function in
// dependency.EnginConfig.
func moreImportant(err, worst error) error {
	return worst
}
