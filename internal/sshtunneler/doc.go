// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshtunneler provides a way to create
// SSH connections to machine units using a reverse
// SSH tunnel approach.
//
// The idea behind this approach is that Juju units require
// connectivity to controllers, but the reverse is not true.
// So, to establish SSH connections to machines, the machines
// create a reverse SSH tunnel to the controller.
//
// The high-level flow is as follows:
// 1. Tunnels are established using the `TunnelTracker`
// object, which allows you to request a tunnel to a machine.
// 2. Requests for tunnels are watched by machines and acted
// upon by connecting to the controller.
// 3. The `TunnelTracker` is used again to authenticate the
// connection and pass the connection to the routine that
// requested it.
//
// See the exported methods on the `TunnelTracker` and
// `TunnelRequest` objects for more information.
package sshtunneler
