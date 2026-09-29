// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package model defines the manifolds for the workers that run for every
// model on a controller.
//
// The IAASManifolds and CAASManifolds functions each construct a
// dependency.Manifolds value: the set of workers run for a model on
// machines, or for a model on Kubernetes, respectively. CAASManifolds is the
// Kubernetes counterpart of IAASManifolds. Both are variations over the
// common set built by commonManifolds.
//
// The machine agent defined in github.com/juju/juju/cmd/jujuagentd/agent
// starts a dependency engine for each model and installs the manifolds
// matching the model type into it. The workers of the machine agent itself
// are declared in github.com/juju/juju/cmd/jujuagentd/agent/machine.
package model
