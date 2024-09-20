// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package computeprovisioner defines the compute provisioner worker. This
// worker is responsible for provisioning new compute (machine) instances in
// the cloud provider when a new machine is added to the model.
//
// This worker, the same as the containerprovisioner, will retrieve the
// machines from the database and simply do the provisioning.
package computeprovisioner
