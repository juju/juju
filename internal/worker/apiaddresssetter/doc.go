// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package apiaddresssetter is a worker which sets the API addresses for the
// each controller node, watching for changes both in the controller node's ip
// addresses and the controller config (the juju-mgmt-space key) to filter the
// addresses based on the management space.
package apiaddresssetter
