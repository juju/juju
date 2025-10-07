// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package blockcommand defines the domain model, contracts, and documentation
// for Juju command blocks.
//
// Command blocks are model-scoped guard rails that temporarily prevent classes
// of user operations and API calls from proceeding. Blocks can carry an optional
// human-readable message explaining why the action is blocked.
//
// The following block types are supported:
//   - DestroyBlock: prevents destruction of the model.
//   - RemoveBlock: prevents removal of model entities such as machines,
//     applications, units, or relations.
//   - ChangeBlock: prevents general model changes (adding, modifying, or
//     removing entities).
//
// The blockcommand domain provides:
//   - Types and validation for BlockType and Block.
//   - A service (package blockcommand/service) offering business logic to switch
//     blocks on and off and to query active blocks and their messages.
//   - A persistence layer (package blockcommand/state) used by the service.
//
// Consumers include client facades and CLI commands that expose the functionality
// to users, such as
//   - apiserver/facades/client/block for remote API access.
//   - api/client/block for client helpers.
//   - cmd/juju/block for CLI commands to enable, list, and remove blocks.
//
// Typical usage:
//   - A client enables a block with an optional message to protect the model
//     during maintenance windows.
//   - System components and user commands check for active blocks before
//     performing protected actions; when blocked, the optional message is
//     surfaced to the caller.
package blockcommand
