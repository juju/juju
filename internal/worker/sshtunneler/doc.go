// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package sshtunneler uses the internal/sshtunneler package
// to provide a singleton instance of a tunnel tracker that
// can be used to co-ordinate the initiation of reverse SSH
// tunnels to the controller, and then perform an another
// SSH connection back down the reverse tunnel.
package sshtunneler
