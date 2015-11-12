// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

// MachineStorageId associates a machine entity with a storage entity. They're
// expressed as tags because they arrived here as a move, not a change; ideally
// a MachineStorageIdsWatcher would return them in a more model-appropriate
// format (i.e. not as strings-that-probably-parse-to-tags).
type MachineStorageId struct {
	MachineTag    string
	AttachmentTag string
}

// MachineStorageIdsChan is a change channel as described in the CoreWatcher
// docs.
//
// Other than that, I don't know its exact semantics. axw, description? standard
// add/remove? changes to referenced entities?
type MachineStorageIdsChan <-chan []MachineStorageId

// MachineStorageIdsWatcher conveniently ties a MachineStorageIdsChan to the
// worker.Worker that represents its validity.
type MachineStorageIdsWatcher interface {
	CoreWatcher
	Changes() MachineStorageIdsChan
}
