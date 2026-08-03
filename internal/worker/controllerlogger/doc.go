// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controllerlogger manages controller-process logging configuration.
//
// Controller-process logging configuration is the effective loggo configuration
// used by the standalone controller process. This package exists for the
// controller-only path, where the controller runs with local controller-owned
// state and should read controller model logging configuration directly rather
// than going back through the machine-agent API caller manifold.
//
// See github.com/juju/juju/internal/worker/logger for the shared non-controller
// logging worker used by other agent types. See
// github.com/juju/juju/internal/worker/domainservices for the controller-local
// service source that supplies the controller model configuration used here.
package controllerlogger
