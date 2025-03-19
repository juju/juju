// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package params contains the definition of the request and response parameters for the Juju API Server and
// Juju API callers (which includes the user-facing clients and the different agents: controller, machine, and unit). It includes:
// - Constants for handling various API cases based on specific values of request fields.
// - Struct definitions for API requests to and responses from the Juju API Server. JSON struct tags facilitate serialization and deserialization for network communication.

// When introducing a change to a facade, to maintain compatibility with a previous version of the facade, copy the existing parameter struct
// and append to its name the version of the compatible facade (e.g., [params.AddApplicationUnitsV5](https://github.com/juju/juju/blob/e746c7c7fbf6d1e9be0807357c5bdcc210ec22d7/rpc/params/params.go#L451)).
// The new facade will use the undecorated name (e.g., [params.AddApplicationUnits](https://github.com/juju/juju/blob/e746c7c7fbf6d1e9be0807357c5bdcc210ec22d7/rpc/params/params.go#L441).

package params
