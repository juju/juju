// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"os"
	"path/filepath"

	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/errors"
)

// RecoveryParamsFile is the path, relative to the agent data directory, of the
// marker written when a replacement controller has been allocated in recovery
// mode. Its presence tells jujuagentd not to start the dependency engine until
// the recovery import completes.
const RecoveryParamsFile = "recovery/params"

// RecoveryAgentWaitingFile records that the agent reached its import fence.
// It is written on the same volume as its configuration so pod restarts can
// recognise a completed recovery setup.
const RecoveryAgentWaitingFile = "recovery/agent-waiting"

// IsRecovery reports whether dataDir holds a recovery parameters marker.
//
// A missing marker means the agent is not a recovery allocation. Any other
// stat error is returned unchanged. A marker that exists but is not a
// regular file is invalid: the marker must be readable as empty recovery
// parameters. The stat does not follow symlinks, so a symlinked marker
// counts as invalid rather than as a present marker.
func IsRecovery(dataDir string) (bool, error) {
	paramsPath := filepath.Join(dataDir, RecoveryParamsFile)
	info, err := os.Lstat(paramsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, errors.Capture(err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.Errorf(
			"invalid recovery params path %q: expected regular file", paramsPath)
	}
	return true, nil
}

// AcknowledgeRecoveryWait records that the running agent observed its fence.
// The acknowledgement survives retries until the recovery directory is removed.
func AcknowledgeRecoveryWait(dataDir string) error {
	path := filepath.Join(dataDir, RecoveryAgentWaitingFile)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return errors.Capture(utils.AtomicWriteFile(path, nil, 0600))
	}
	if err != nil {
		return errors.Capture(err)
	}
	if !info.Mode().IsRegular() {
		return errors.Errorf("invalid recovery acknowledgement %q: expected regular file", path)
	}
	return nil
}
