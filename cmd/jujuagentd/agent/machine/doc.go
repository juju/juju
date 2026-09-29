// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package machine defines the manifolds for the machine agent.
//
// The IAASManifolds and K8sManifolds functions each construct a
// dependency.Manifolds value: the set of workers the machine agent runs when
// it is a machine in a machine deployment, or the controller container in a
// Kubernetes deployment, respectively. Both are variations over the common
// set built by commonManifolds.
//
// The manifolds are installed into the agent's dependency engine by the
// machine agent defined in github.com/juju/juju/cmd/jujuagentd/agent, which
// selects between the two according to whether it runs in a Kubernetes
// deployment. The workers that run per model are declared in
// github.com/juju/juju/cmd/jujuagentd/agent/model.
package machine
