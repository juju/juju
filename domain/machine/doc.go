// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package machine provides the services for managing machines in Juju.
//
// Machines in Juju represent compute resources (physical or virtual) that can
// host Juju units. The machine domain manages the lifecycle, configuration, and
// state of machines within a model.
//
// # Key Concepts
//
// Machines can be:
//   - IAAS machines: virtual or physical machines in cloud providers
//   - Manual machines: pre-existing machines added to Juju
//   - Containers: LXD or KVM containers hosted on other machines
//   - Controller machines: machines hosting Juju controller processes
//
// Each machine has:
//   - Hardware characteristics (memory, CPU, disk, etc.)
//   - Cloud instance information (instance ID, availability zone)
//   - Network configuration (addresses, opened ports)
//   - Status and lifecycle management
//   - Agent version information
//
// # Machine Lifecycle
//
// The lifecycle of a machine is managed by the life.Life type, with states:
// alive, dying, and dead. Machine removal and deletion are coordinated with
// the removal domain to ensure proper cleanup.
package machine
