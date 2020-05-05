// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/juju/core/paths/transientfile"
	"github.com/juju/names/v4"
)

// Monitor leverages juju's transient file mechanism to deliver one-off
// machine reboot notifications to interested entities.
type Monitor struct {
	transientDir string
}

// NewMonitor returns a reboot monitor instance that stores its internal state
// into transientDir.
func NewMonitor(transientDir string) *Monitor {
	return &Monitor{
		transientDir: filepath.Join(transientDir, "reboot-monitor"),
	}
}

// Check for a pending reboot notification for the specified tag. Once
// the notification is consumed, future calls to Check will always
// return false.
func (m *Monitor) Query(tag names.Tag) (bool, error) {
	flagFile := tag.String()
	flagPath := filepath.Join(m.transientDir, flagFile)

	// If the flag file is present, we have already delivered a reboot
	// notification.
	if stat, err := os.Stat(flagPath); err == nil {
		if stat.IsDir() {
			return false, errors.Errorf("expected reboot monitor flag %q to be a file", flagPath)
		}

		return false, nil
	}

	// Create a transient file to capture the flag and return back a
	// reboot notification to the caller.
	f, err := transientfile.Create(m.transientDir, flagFile)
	if err != nil {
		return false, errors.Annotatef(err, "creating reboot monitor flag file %q", flagPath)
	}

	_ = f.Close()
	return true, nil
}

// PurgeState deletes any internal state maintained by the monitor for a
// particular entity.
func (m *Monitor) PurgeState(tag names.Tag) error {
	flagPath := filepath.Join(m.transientDir, tag.String())
	return os.Remove(flagPath)
}
