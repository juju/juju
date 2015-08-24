// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"github.com/juju/errors"
)

// EventsClosed indicates that no more events may be added.
var EventsClosed = errors.New("events closed")
