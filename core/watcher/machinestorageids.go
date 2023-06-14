// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

// MachineStorageID associates a machine entity with a storage entity. They're
// expressed as tags because they arrived here as a move, not a change; ideally
// a MachineStorageIDsWatcher would return them in a more model-appropriate
// format (i.e. not as strings-that-probably-parse-to-tags).
type MachineStorageID struct {
	MachineTag    string
	AttachmentTag string
}

// MachineStorageIDsChannel is a change channel as described in the CoreWatcher
// docs.
//
// It reports additions and removals to a set of attachments; and lifecycle
// changes within the active set.
// This is deprecated; use <-chan []MachineStorageID instead.
type MachineStorageIDsChannel = <-chan []MachineStorageID

// MachineStorageIDsWatcher reports additions and removals to a set of
// attachments; and lifecycle changes within the active set.
type MachineStorageIDsWatcher = Watcher[[]MachineStorageID]
