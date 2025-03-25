// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import "github.com/juju/juju/internal/errors"

const ErrSubscriptionClosed = errors.ConstError("watcher subscription closed")
