// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package model contains the controller model service.
//
// The controller stores information about all the models it manages. This
// service exposes methods to perform management operations such as creating,
// destroying and gathering information about models the controller manages.
//
// # Model Fundamentals
//
// A model in Juju represents an environment where applications are deployed
// and managed. Each model:
//   - Runs on a specific cloud and region
//   - Has its own configuration and credentials
//   - Contains applications, units, machines, and relations
//   - Is owned by a user and can have additional authorized users
//   - Has a unique UUID and name within the controller
//
// # Model Types
//
// Models can be:
//   - IAAS models: deployed on traditional infrastructure (AWS, Azure, etc.)
//   - CAAS models: deployed on Kubernetes clusters
//
// # Model Lifecycle
//
// Models progress through lifecycle states:
//   - Creation: model is initialized with configuration
//   - Active: model is operational and can host applications
//   - Destroying: model and its resources are being torn down
//   - Dead: model has been completely removed
//
// Models can also be migrated between controllers, suspending operations
// during the migration process.
package model
