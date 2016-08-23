// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
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
	caller, err := r.Root.FindMethod(facadeName, version, methodName)
	if err != nil {
		return nil, err
	}
	if err := r.check(facadeName, methodName); err != nil {
		return nil, err
	}
	return caller, nil
}

// restrictRootEarly wraps the provided root so that the check
// function is called on all method lookups. If the check returns an
// error the API call is blocked. This differs from restrictRoot in
// that the check is done before any lookups on the underlying root.
func restrictRootEarly(root rpc.Root, check func(string, string) error) *restrictedEarlyRoot {
	return &restrictedEarlyRoot{
		Root:  root,
		check: check,
	}
}

type restrictedEarlyRoot struct {
	rpc.Root
	check func(facadeName, methodName string) error
}

// FindMethod implements rpc.Root.
func (r *restrictedEarlyRoot) FindMethod(facadeName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if err := r.check(facadeName, methodName); err != nil {
		return nil, err
	}
	return r.Root.FindMethod(facadeName, version, methodName)
}
