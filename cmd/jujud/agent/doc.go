// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package agent provides the command implementations for jujud -- the Juju
// controller application. jujud is the server-side workload that runs on
// controller machines; it is not an agent in the traditional Juju sense.
// Machine and unit agents live in jujuagentd instead.
//
// The package name "agent" is a historical artefact and does not reflect the
// role of the code it contains. jujud is the controller application, not an
// agent -- it is the workload that hosts the API server, database accessor,
// model worker manager, and all other controller services via a dependency
// engine. Several specialised modes also live here: the safe-mode application
// starts only the minimum set of workers needed to bring Dqlite online for
// database recovery, and the DB REPL application provides an interactive
// read-eval-print loop for direct database access. The bootstrap command
// initialises a new controller's state from staged configuration, and the init
// command delivers snap-private files into the confined snap directory tree
// before bootstrap runs.
//
// Consider renaming this package to something that reflects its role as the
// controller application rather than an agent.
//
// See the subpackages below for the dependency manifold definitions that each
// application installs. See cmd/jujud/agent/controller for the full controller
// application manifolds. See cmd/jujud/agent/model for model worker manifolds
// used by every model in a controller. See cmd/jujud/agent/dbrepl and
// cmd/jujud/agent/safemode for the specialised application manifolds. See
// github.com/juju/juju/agent for agent configuration and identity. See
// github.com/juju/juju/internal/worker for the worker implementations used in
// the manifolds.
package agent
