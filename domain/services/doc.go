// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package services provides functionality to manage services. Service types
// (controller, model, provider and object store) wrap database access. Services
// encapsulate domain verticals, which can be used to encapsulate business and
// persistence logic.
//
// The controller service wraps the global "controller" namespace database.
// Everything in the controller namespace is global and shared across all
// models. It can generally be accessed by any consumer of the model domain
// services (referred to as "domain services").
//
// The model domain services ("domain services") wraps a model namespace
// database. The model namespace is specific to a particular model. The model
// domain services can be accessed by any the knowledge of the namespace.
//
// The provider services is a special case, whereby it takes a very small subset
// of the controller and the model domain services and offers methods for the
// sole purpose of managing providers (environs and brokers). The provider
// services has very few dependencies and is only used by the provider tracker.
// The provider tracker caches providers for both IAAS and CAAS model types.
// Ensuring that any model configuration, cloud configuration or credential
// changes update the cached providers, without the need to restart the
// controller.
//
// In order to avoid circular dependencies, the object store services is
// separated from the domain services. The object store services wraps the
// object store namespace database. The object store namespace is specific to a
// particular model. The domain services can consume the object store services
// without the worry of circular dependencies.
//
// The domain services can therefore consume additional dependencies from other
// dependency engine outputs without the worry of circular dependencies.
//
//
//                            ┌────────────────┐
//                            │                │
//                            │                │
//                            │   DBACCESSOR   │
//                            │                │
//                            │                │
//                            └───────┬────────┘
//                                    │
//                 ┌──────────────────┼────────────────┐
//                 │                  │                │
//         ┌───────▼───────┐          │        ┌───────▼────────┐
//         │               │          │        │                │
//         │   PROVIDER    │          │        │                │
//         │   SERVICES    │          │        │   OBJECTSTORE  │
//         │               │          │        │   SERVICES     │
//         └───────┬───────┘          │        │                │
//                 │                  │        │                │
//                 │                  │        └───────┬────────┘
//                 │                  │                │
//       ┌─────────▼─────┐            │                │
//       │               │            │                │
//       │ ┌───────────────┐          │                │
//       │ │               │          │        ┌───────▼─────────┐
//       │ │  ┌───────────────┐       │        │                 │
//       │ │  │               │       │        │                 │
//       │ │  │               │       │        │                 │
//       └─│  │   PROVIDER    │       │        │   OBJECTSTORE   │
//         │  │   TRACKER(S)  │       │        │                 │
//         └──│               │       │        │                 │
//            │               │       │        │                 │
//            └────┬──────────┘       │        └───────┬─────────┘
//                 │                  │                │
//                 └────────────────┐ │ ┌──────────────┘
//                                  │ │ │
//                           ┌──────▼─▼─▼──┐
//                           │             │
//                           │             │
//                           │   DOMAIN    │
//                           │   SERVICES  │
//                           │             │
//                           │             │
//                           └──────┬──────┘
//                                  │
//                                  │
//                           ┌──────▼──────┐
//                           │             │
//                           │             │
//                           │   OUTPUT    │
//                           │             │
//                           │             │
//                           └─────────────┘
//

package services
