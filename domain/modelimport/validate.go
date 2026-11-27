// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport

import (
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/export/types/latest"
	"github.com/juju/juju/internal/errors"
)

// ValidatePayload validates target-version payload invariants that must hold
// before any target-side import writes proceed.
func ValidatePayload(payload latest.ModelExport) error {
	if len(payload.ModelAgent) != 1 {
		return errors.Errorf(
			"model export payload has %d model_agent rows, expected 1: %w",
			len(payload.ModelAgent), coreerrors.NotValid)
	}
	type filesystemNode struct {
		filesystem string
		node       string
	}
	attachments := make(map[filesystemNode]string)
	for _, attachment := range payload.StorageFilesystemAttachment {
		key := filesystemNode{
			filesystem: attachment.StorageFilesystemUUID,
			node:       attachment.NetNodeUUID,
		}
		if previous, ok := attachments[key]; ok {
			return errors.Errorf(
				"filesystem %q has duplicate attachments %q and %q on network node %q; resolve the duplicate on the source model before retrying: %w",
				key.filesystem, previous, attachment.UUID, key.node, coreerrors.NotValid,
			)
		}
		attachments[key] = attachment.UUID
	}
	return nil
}
