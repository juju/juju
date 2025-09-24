// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state provides state management for applications.
//
// There are three types of application:
//
//   - IAAS applications, which are deployed to machines.
//   - CAAS applications, which are deployed to Kubernetes clusters.
//   - Synthetic Cross-Model-Relation (CMR) applications, which represent
//     applications in other models that are related to applications in this
//     model via cross-model relations.
//
// Each application has a life cycle, represented by the life.Life type. The
// life, thus the removal and deletion of the entities are managed by the
// removal domain.
//
// CMR applications are managed by the crossmodelrelation domain.

package state
