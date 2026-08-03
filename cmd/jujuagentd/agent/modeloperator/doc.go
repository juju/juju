// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package modeloperator provides the agent configuration for a Juju model
// operator running inside a CAAS (Kubernetes) cluster.
//
// A model operator is a dedicated Juju agent deployed as a pod within the
// Kubernetes namespace allocated to a CAAS model -- one operator per model.
// It acts as the in-cluster representative of the model, bridging the Juju
// controller (which may be external to the cluster) with the Kubernetes
// namespace where the model's application workloads run. The model operator
// carries the model's own agent identity, separate from unit and application
// agents, and is provisioned automatically by the controller whenever a CAAS
// model is active.
//
// See github.com/juju/juju/internal/worker/caasmodeloperator for the
// controller-side worker that provisions model operator deployments in
// Kubernetes. See github.com/juju/juju/caas for the broker interface that
// manages the model operator lifecycle.
package modeloperator
