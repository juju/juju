// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package juju provides client-side utilities for connecting to Juju.
//
// These utilities establish authenticated API connections to Juju controllers
// (enabling CLI commands and other clients to interact with controllers and
// controllers and models) and initialize the Juju client environment
// (preparing the Juju data directory and SSH configuration before CLI
// commands run).
//
// See github.com/juju/juju/api for the underlying connection primitives.
// See github.com/juju/juju/api/jujuclient for controller store management.
// See github.com/juju/juju/cmd/juju/commands for CLI initialization patterns.
package juju
