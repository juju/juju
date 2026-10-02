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
	// SnapInitDir is the directory under $SNAP_COMMON where the configure hook
	// stores deferred snap-config overlay state that was set before runtime.conf
	// exists.
	SnapInitDir = ".snap-init"

	// deferredLoggingOverrideFile is the name of the file that persists the
	// deferred logging-override snap config value.
	deferredLoggingOverrideFile = "logging-override"
)

// SupportedSnapConfigKeys is the explicit allowlist of supported snap-config
// keys. Only these keys may be applied through the snap configure hook and
// overlay helper. Keys not listed here are rejected with a clear error.
var SupportedSnapConfigKeys = []string{
	"logging-override",
}

// ValidateSnapConfigOverlay checks that a set of snap-config key-value pairs
// contains only supported keys. It returns an error if any
// controller-database-owned key is present, or if any key is not listed in
// SupportedSnapConfigKeys.
func ValidateSnapConfigOverlay(vals map[string]string) error {
	var unsupported []string
	for k := range vals {
		if !isSupportedSnapConfigKey(k) {
			unsupported = append(unsupported, k)
		}
	}
	if len(unsupported) > 0 {
		return errors.Errorf(
			"cannot apply snap config: %d unsupported key(s) are not supported through snap set: %v",
			len(unsupported),
			unsupported,
		)
	}
	return nil
}

func isSupportedSnapConfigKey(key string) bool {
	return slices.Contains(SupportedSnapConfigKeys, key)
}

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
	if err := utils.AtomicWriteFile(path, []byte(value+"\n"), 0o600); err != nil {
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
// This function is the exclusive mutation path for snap-config
// overlays. The SnapConfigOverlay struct acts as a compile-time allowlist:
// only fields present on it can be mutated through this path.
func ApplySnapConfigOverlay(runtimeConfigPath string, overlay SnapConfigOverlay) error {
	return ChangeControllerRuntimeConfig(runtimeConfigPath, func(cfg *ControllerRuntimeConfig) error {
		cfg.LoggingOverride = overlay.LoggingOverride
		return nil
	})
}
