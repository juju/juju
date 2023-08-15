// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	"github.com/juju/rpcreflect"

	"github.com/juju/juju/rpc"
)

// restrictRoot wraps the provided root so that the check function is
// called on all method lookups. If the check returns an error the API
// call is blocked.
func restrictRoot(root rpc.Root, check func(string, string) error) *restrictedRoot {
	return &restrictedRoot{
		Root:  root,
		check: check,
	}
}

type restrictedRoot struct {
	rpc.Root
	check func(facadeName, methodName string) error
}

// FindMethod implements rpc.Root.
func (r *restrictedRoot) FindMethod(facadeName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if err := r.check(facadeName, methodName); err != nil {
		return nil, err
	}
	return r.Root.FindMethod(facadeName, version, methodName)
}

// StartTrace implements rpc.Root.
func (r *restrictedRoot) StartTrace(ctx context.Context, name string) (context.Context, rpc.Span) {
	return r.Root.StartTrace(ctx, name)
}

// restrictAll blocks all API requests, returned a fixed error.
func restrictAll(root rpc.Root, err error) *restrictedRoot {
	return restrictRoot(root, func(string, string) error {
		return err
	})
}
