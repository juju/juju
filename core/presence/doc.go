// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package presence works on the premise that an agent it alive
// if it has a current connection to one of the API servers.
//
// This package handles all of the logic around collecting an organising
// the information around all the connections made to the API servers.
package presence
