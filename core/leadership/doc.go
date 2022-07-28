// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package leadership holds code pertaining to application leadership in juju. It's
// expected to grow as we're able to extract (e.g.) the Ticket and Tracker
// interfaces from worker/leadership; and quite possible the implementations
// themselves; but that'll have to wait until it can all be expressed without
// reference to non-core code.
package leadership
