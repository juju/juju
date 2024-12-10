// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service contains the services required for interacting wit the
// underlying models within a Juju controller. This includes the management of
// model's and the respective information for each model.
//
// # Model Status
// A model maintains and can be in exactly one status state at any given time.
// We consider a model's status as purely a user interface value and not a
// programmatic one that should be used for gating decisions on with the
// business logic of the controller.
//
// A model can be in one of the following states:
// - Available: The model is fully operational.
// - Suspended: The model's cloud credential is considered invalid and the model
// is unable to perform operations on the cloud/provider.
// - Destroying: The model and it's resources are being destroyed and are about
// to go away.
// - Busy: The model is currently being migrated to another controller or the
// model is being migrated into the current controller.
package service
