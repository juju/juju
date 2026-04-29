// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package caas provides abstractions for container orchestration platforms.
//
// These abstractions enable Juju to deploy and manage applications on
// Kubernetes and other container platforms via the Broker interface (providing
// application lifecycle operations, storage management, networking
// configuration, model operator deployment, etc.). Applications are deployed
// with configurable deployment types (stateless, stateful, daemon) and service
// types (cluster, load balancer, external, etc.). The ContainerEnvironProvider
// interface extends the standard environ provider to support container-specific
// operations.
//
// See github.com/juju/juju/environs for the base environment provider
// interface. See github.com/juju/juju/internal/provider/kubernetes for the
// Kubernetes implementation. See github.com/juju/juju/internal/worker/caasapplicationprovisioner
// for application provisioning using brokers.
package caas
