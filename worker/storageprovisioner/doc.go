// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package storageprovisioner provides a worker that manages the provisioning
// and deprovisioning of storage volumes and filesystems, and attaching them
// to and detaching them from machines.
//
// A storage provisioner worker is run at each model manager, which
// manages model-scoped storage such as virtual disk services of the
// cloud provider. In addition to this, each machine agent runs a machine-
// storage provisioner worker that manages storage scoped to that machine,
// such as loop devices, temporary filesystems (tmpfs), and rootfs.
//
// The storage provisioner worker is comprised of the following major
// components:
//   - a set of watchers for provisioning and attachment events
//   - a schedule of pending operations
//   - event-handling code fed by the watcher, that identifies
//     interesting changes (unprovisioned -> provisioned, etc.),
//     ensures prerequisites are met (e.g. volume and machine are both
//     provisioned before attachment is attempted), and populates
//     operations into the schedule
//   - operation execution code fed by the schedule, that groups
//     operations to make bulk calls to storage providers; updates
//     status; and reschedules operations upon failure
package storageprovisioner
