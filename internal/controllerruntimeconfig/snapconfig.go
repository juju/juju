// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerruntimeconfig

import (
	"os"
	"path/filepath"
	"slices"
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

// SupportedSnapConfigKeys is the explicit allowlist of snap-config keys
// that are supported in Phase 1. Only these keys may be applied through
// the snap configure hook and overlay helper.
var SupportedSnapConfigKeys = []string{
	"logging-override",
}

// UnsupportedDBSnapConfigKeys is the list of controller-database-owned
// runtime config keys that must NOT be set through snap config in Phase 1.
// These keys are seeded from controller config at bootstrap and stay under
// controller-database ownership after bootstrap. Snap config must not create
// a second competing source of truth for them.
var UnsupportedDBSnapConfigKeys = []string{
	"agent-logfile-max-size-mb",
	"agent-logfile-max-backups",
	"query-tracing-enabled",
	"query-tracing-threshold",
	"dqlite-busy-timeout",
}

// IsSupportedSnapConfigKey reports whether the given key is in the
// Phase 1 allowlist.
func IsSupportedSnapConfigKey(key string) bool {
	return slices.Contains(SupportedSnapConfigKeys, key)
}

// IsUnsupportedDBSnapConfigKey reports whether the given key is a
// known controller-database-owned key that must not be set through
// snap config. This is used by the configure hook to reject ownership
// violations before any runtime.conf mutation.
func IsUnsupportedDBSnapConfigKey(key string) bool {
	return slices.Contains(UnsupportedDBSnapConfigKeys, key)
}

// ErrUnsupportedSnapConfigKey is returned when a controller-database-owned
// key appears in snap config.
func ErrUnsupportedSnapConfigKey(key string) error {
	return errors.Errorf(
		"unsupported snap-config key %q: this key is controller-database-owned and not exposed through snap set in Phase 1",
		key,
	)
}

// ValidateSnapConfigOverlay checks that a set of snap-config key-value pairs
// contains only supported Phase 1 keys. It returns an error if any
// controller-database-owned key is present.
func ValidateSnapConfigOverlay(vals map[string]string) error {
	var unsupported []string
	for k := range vals {
		if IsUnsupportedDBSnapConfigKey(k) {
			unsupported = append(unsupported, k)
		}
	}
	if len(unsupported) > 0 {
		return errors.Errorf(
			"cannot apply snap config: %d controller-database-owned key(s) "+
				"are not supported through snap set in Phase 1: %v",
			len(unsupported),
			unsupported,
		)
	}
	return nil
}

// DeferredLoggingOverridePath returns the path to the deferred
// logging-override state file under the given snapCommon directory.
func DeferredLoggingOverridePath(snapCommon string) string {
	return filepath.Join(snapCommon, SnapInitDir, deferredLoggingOverrideFile)
}

// ErrInvalidSnapConfigValue is returned when a snap-config value does not
// meet the validation rules for its key.
func ErrInvalidSnapConfigValue(key string, reason error) error {
	return errors.Errorf("invalid snap-config value for %q: %v", key, reason)
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
//
// This function is the exclusive Phase 1 mutation path for snap-config
// overlays. The SnapConfigOverlay struct acts as a compile-time allowlist:
// only fields present on it can be mutated through this path.
func ApplySnapConfigOverlay(runtimeConfigPath string, overlay SnapConfigOverlay) error {
	return ChangeControllerRuntimeConfig(runtimeConfigPath, func(cfg *ControllerRuntimeConfig) error {
		cfg.LoggingOverride = overlay.LoggingOverride
		return nil
	})
}
