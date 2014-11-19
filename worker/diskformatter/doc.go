// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package diskformatter defines a worker that watches for block devices
// attached to datastores owned by the unit that runs this worker, and
// creates filesystems on them as necessary. Each unit agent runs this
// worker.
package diskformatter
