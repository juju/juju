// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package containerprovisioner defines the container provisioner worker. This
// worker is responsible for provisioning new containers on the underlying
// machine.
//
// This worker, the same as the computeprovisioner, will retrieve the
// containers from the database and simply do the provisioning.
package containerprovisioner
