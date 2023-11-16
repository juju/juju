// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package caasapplicationprovisioner defines two types of worker:
//   - provisioner: Watches a Kubernetes model and starts a new worker
//     of the appWorker type whenever an application is created.
//   - appWorker: Drives the Kubernetes provider to create, manage,
//     and destroy Kubernetes resources to match a requested state. Also
//     writes the state of created resources (application/unit status,
//     application/unit IP addresses & ports, filesystem info, etc.)
//     back into the database.
package caasapplicationprovisioner
