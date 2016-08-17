// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/rpcreflect"
)

// The controllerFacadeNames are the root names that can be accessed
// using a controller-only login. Any facade added here needs to work
// independently of individual models.
var controllerFacadeNames = set.NewStrings(
	"AllModelWatcher",
	"Cloud",
	"Controller",
	"MigrationTarget",
	"ModelManager",
	"UserManager",
)

func isControllerFacade(facadeName string) bool {
	// Note: the Pinger facade can be used in both model and controller
	// connections.
	return controllerFacadeNames.Contains(facadeName) || facadeName == "Pinger"
}

func isModelFacade(facadeName string) bool {
	return !controllerFacadeNames.Contains(facadeName)
}

// restrictedRoot restricts API calls to facades that match a given
// predicate function.
type restrictedRoot struct {
	rpc.Root
	allow    func(facadeName string) bool
	connType string
}

// newRestrictedRoot returns a new restrictedRoot that allows all facades
// served by the given finder for which allow(facadeName) returns true.
//
// The connType value should hold the type of the API connection - it's
// used to make a nicer error message when access is denied.
func newRestrictedRoot(root rpc.Root, connType string, allow func(string) bool) *restrictedRoot {
	return &restrictedRoot{
		Root:     root,
		allow:    allow,
		connType: connType,
	}
}

// FindMethod returns a not supported error if the rootName is not one
// of the facades available at the server root when there is no active
// environment.
func (r *restrictedRoot) FindMethod(rootName string, version int, methodName string) (rpcreflect.MethodCaller, error) {
	if !r.allow(rootName) {
		return nil, errors.NewNotSupported(nil, fmt.Sprintf("facade %q not supported for %s API connection", rootName, r.connType))
	}
	caller, err := r.Root.FindMethod(rootName, version, methodName)
	if err != nil {
		return nil, err
	}
	return caller, nil
}
