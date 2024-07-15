// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

// ## Block Type
//
// Model level Blocks are used to prevent accidental damage to Juju deployments.
// Blocks can be switched on/off to prevent running some operations.
//
//  1. DestroyBlock type identifies block that prevents model destruction.
//  2. RemoveBlock type identifies block that prevents removal of machines,
//     applications, units or relations.
//  3. ChangeBlock type identifies block that prevents model changes such as
//     additions, modifications, removals of model entities.
