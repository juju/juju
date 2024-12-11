// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package service contains the services required for interacting with the
// underlying models within a Juju controller. This includes the management of
// models and the respective information for each model.
//
// # Model Status
//
// We consider a model's status as purely a user interface value and not a
// programmatic one that should be used for informing the operation of
// the business logic of the controller.
//
// A model can be in one of the following states:
// - Available: the model is fully operational.
// - Suspended: the model's cloud credential is considered invalid and the model
// is unable to perform operations on the cloud/provider.
// - Destroying: the model and its resources are being destroyed and are about
// to go away.
// - Busy: the model is currently being migrated to another controller or the
// model is being migrated into the current controller.
package service
