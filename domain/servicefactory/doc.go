// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package servicefactory provides functionality to manage service factories.
// Service types (controller, model and provider) wrap database access. Services
// encapsulate domain verticals, which can be used to encapsulate business and
// persistence logic.
//
// The controller service wraps the global "controller" namespace database.
// Everything in the controller namespace is global and shared across all
// models. It can generally be accessed by any consumer of the model service
// factory (referred to as "service factory").
//
// The model service factory ("service factory") wraps a model namespace
// database. The model namespace is specific to a particular model. The model
// service factory can be accessed by any the knowledge of the namespace.
//
// The provider service factory is a special case, whereby it takes a very
// small subset of the controller and the model service factories and offers
// methods for the sole purpose of managing providers (environs and brokers).
// The provider service factory has very few dependencies and is only used
// by the provider tracker. The provider tracker caches providers for both
// IAAS and CAAS model types. Ensuring that any model configuration, cloud
// configuration or credential changes update the cached providers, without
// the need to restart the controller.
//
// The service factory can therefore consume additional dependencies from other
// dependency engine outputs without the worry of circular dependencies.
//
//                             ┌────────────────┐
//                             │                │
//                             │                │
//                             │   DBACCESSOR   │
//                             │                │
//                             │                │
//                             └───────┬────────┘
//                                     │
//                  ┌──────────────────┤
//                  │                  │
//          ┌───────▼───────┐          │
//          │               │          │
//          │   PROVIDER    │          │
//          │   SERVICE     │          │
//          │   FACTORY     │          │
//          │               │          │
//          └───────┬───────┘          │
//                  │                  │
//                  │                  │
//                  │                  │
//        ┌─────────▼─────┐            │
//        │               │            │
//        │ ┌───────────────┐          │
//        │ │               │          │
//        │ │  ┌───────────────┐       │
//        │ │  │               │       │
//        │ │  │               │       │
//        └─│  │   PROVIDER    │       │
//          │  │   TRACKER(S)  │       │
//          └──│               │       │
//             │               │       │
//             └────┬──────────┘       │
//                  │                  │
//                  └────────────────┐ │
//                                   │ │
//                            ┌──────▼─▼────┐
//                            │             │
//                            │             │
//                            │   SERVICE   │
//                            │   FACTORY   │
//                            │             │
//                            │             │
//                            └──────┬──────┘
//                                   │
//                                   │
//                            ┌──────▼──────┐
//                            │             │
//                            │             │
//                            │   OUTPUT    │
//                            │             │
//                            │             │
//                            └─────────────┘

package servicefactory
