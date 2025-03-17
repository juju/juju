// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package removal implements the entity removal domain.
// In general removing an entity involves:
//  1. Advancing its life from Alive to Dying. This is noticed by invested
//     workers who then set about processing this fact and preparing other parts
//     of the model for the entity's ultimate deletion. The completion of those
//     activities is represented in the entity's life transitioning further to
//     Dead, whereupon it is no longer a participant in the model.
//  2. Scheduling a removal job, which deletes the data associated with Dead
//     Entities. For jobs allowing a force flag, passing true for said flag will
//     indicate that the removal job should be progressed even should the entity
//     not yet have reached the Dead state.
package removal
