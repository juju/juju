// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modeloperator defines the manifolds for the modeloperator
// agent: the set of workers that run the modeloperator pod in a
// Kubernetes deployment. The Manifolds function constructs the
// dependency.Manifolds value, and the agent definition in
// cmd/jujud/agent (model.go) installs it into the agent's dependency
// engine.
package modeloperator
