// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package controllerruntimeconfig manages the controller runtime configuration
// file written at bootstrap time.
//
// Controller runtime configuration is a local YAML file
// (<controller-agent-dir>/runtime.conf) that holds the static startup values
// required by the controller process and Dqlite before the domain database is
// available. Its fields include the controller ID, data and log directories,
// Dqlite port and tuning settings, the TLS CA certificate, and the controller
// TLS certificate and private key.
//
// The file is distinct from the machine-agent config file (agent.conf) and
// from the charm-written cluster config (controller.conf). It is written once
// at bootstrap with owner-only permissions (0600) because it contains TLS
// private key material.
//
// See github.com/juju/juju/internal/cloudconfig for the IAAS bootstrap path
// that writes this file. See
// github.com/juju/juju/internal/provider/kubernetes for the CAAS bootstrap
// path that projects it into the controller pod via a ConfigMap volume.
package controllerruntimeconfig
