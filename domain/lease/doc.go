// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package lease provides the service for creating and tracking leases.
//
// Leases are used in Juju to track responsibility for tasks that need a
// singular owner. This can be required in many places where there are multiple
// equivalent instances of a process. For example, when juju is running in high
// availability mode with multiple controllers, there should only be one machine
// provisioner per model. The leases can be used to ensure only one of the
// controller instances runs the provisioner for a given model.

package lease
