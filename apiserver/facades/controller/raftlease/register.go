// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("RaftLease", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*APIv2)(nil)))
}

// newFacade create a facade for handling raft leases.
func newFacadeV2(context facade.Context) (*APIv2, error) {
	api, err := NewFacade(context)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}
