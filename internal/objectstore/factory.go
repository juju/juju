// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import "context"

// ObjectStoreWorkerFunc is the function signature for creating a new object
// store worker.
type ObjectStoreWorkerFunc func(context.Context, string, MongoSession, Logger) (TrackedObjectStore, error)
