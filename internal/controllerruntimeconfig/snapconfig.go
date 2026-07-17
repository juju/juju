// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4"
)

const (
	// SnapInitDir is the directory under $SNAP_COMMON where the configure
	// hook stores deferred snap-config overlay state that was set before
	// runtime.conf exists.
	SnapInitDir = ".snap-init"

	// deferredLoggingOverrideFile is the name of the file that persists
	// the deferred logging-override snap config value.
	deferredLoggingOverrideFile = "logging-override"
)

// DeferredLoggingOverridePath returns the path to the deferred
// logging-override state file under the given snapCommon directory.
func DeferredLoggingOverridePath(snapCommon string) string {
	return filepath.Join(snapCommon, SnapInitDir, deferredLoggingOverrideFile)
}

// SnapConfigOverlay holds the snap-set-controlled runtime keys that are
// layered on top of the bootstrap-authored runtime.conf. Currently only
// logging-override is supported.
type SnapConfigOverlay struct {
	LoggingOverride string
}

// ReadDeferredLoggingOverride reads a previously deferred logging-override
// value from the given snapCommon directory. If the file does not exist it
// returns "" with no error.
func ReadDeferredLoggingOverride(snapCommon string) (string, error) {
	path := DeferredLoggingOverridePath(snapCommon)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", errors.Annotatef(err, "reading deferred logging-override %q", path)
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteDeferredLoggingOverride writes the given value to the deferred
// logging-override state file. An empty value deletes the file, clearing the
// deferred override. The parent directory is created if it does not exist.
func WriteDeferredLoggingOverride(snapCommon, value string) error {
	path := DeferredLoggingOverridePath(snapCommon)
	if value == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return errors.Annotatef(err, "removing deferred logging-override %q", path)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errors.Annotatef(err, "creating snap-init directory")
	}
	if err := utils.AtomicWriteFile(path, []byte(value+"\n"), 0o644); err != nil {
		return errors.Annotatef(err, "writing deferred logging-override %q", path)
	}
	return nil
}

// ApplySnapConfigOverlay reads the existing runtime.conf at path, applies
// only the logging-override snap key, and atomically writes the result back.
// All other fields are preserved unchanged.
//
// When runtime.conf does not exist it returns an error that can be detected
// with errors.Is(err, os.ErrNotExist). The caller is expected to defer the
// value in that case rather than fabricating a replacement file.
func ApplySnapConfigOverlay(runtimeConfigPath string, overlay SnapConfigOverlay) error {
	return ChangeControllerRuntimeConfig(runtimeConfigPath, func(cfg *ControllerRuntimeConfig) error {
		cfg.LoggingOverride = overlay.LoggingOverride
		return nil
	})
}
