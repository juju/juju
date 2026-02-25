// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"strings"

	"github.com/juju/description/v11"
	"github.com/juju/names/v6"

	"github.com/juju/juju/internal/errors"
)

// ExtractApplicationUUIDFromRemoteEntities extracts application UUIDs from the
// remote entities in the given model description. It returns a map of
// application UUIDs to their corresponding tokens.
func ExtractApplicationUUIDFromRemoteEntities(model description.Model) (map[string]string, error) {
	remoteEntities := make(map[string]string)
	for _, re := range model.RemoteEntities() {
		// Handle only remote entities that are application UUIDs.
		remoteEntityID := re.ID()
		if !strings.HasPrefix(remoteEntityID, "application-") {
			continue
		}

		tag, err := names.ParseApplicationTag(remoteEntityID)
		if err != nil {
			return nil, errors.Errorf("parsing application tag from remote entity id %q: %w", remoteEntityID, err)
		}

		remoteEntities[tag.Id()] = re.Token()
	}
	return remoteEntities, nil
}
