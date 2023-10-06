// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The upgrade state package provides a type for representing the state of an
// upgrade.
//
// The states are represented as a finite state machine, with the following
// transitions:
//
//	Created -> Started -> DBCompleted -> StepsCompleted
//
// The state machine is represented by the State type, which is an integer
// type. The constants for the states are exported, and the type is exported
// so that it can be used in APIs.
package state
