// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package model defines core model types and identifiers.
// See the sections below for details about this package.
//
// See github.com/juju/juju/core/life for model lifecycle states.
// See github.com/juju/juju/core/credential for model credential management.
//
// # How this package works
//
// **Models**: Models are isolated environments for deploying and managing
// applications. Each model has a unique UUID, a qualified name (name + owner
// qualifier), and a type (IAAS for infrastructure-as-a-service or CAAS for
// container-as-a-service). Models track their target agent version, cloud
// configuration (cloud name, type, region), and lifecycle state (alive, dying,
// dead).
//
// **Model types**: IAAS models deploy applications to machines managed by Juju.
// CAAS models deploy applications to Kubernetes clusters. The model type
// determines the provider implementation and the available operations.
//
// **Model identification**: Models can be addressed by UUID (universal across
// controllers) or by qualified name (owner/name within a controller). The
// qualifier is typically the owner's username. Short UUIDs (first 6 characters)
// are used in CLI output for brevity.
//
// # How to use this package correctly
//
// **Validation**: Model UUIDs MUST be validated using UUID.Validate() before
// persistence. Model qualifiers MUST be valid user identifiers as defined by
// names.IsValidUser(). ModelType values MUST be either IAAS or CAAS.
package model
