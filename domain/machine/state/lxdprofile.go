// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/machine/internal"
	"github.com/juju/juju/internal/errors"
)

// GetLXDProfilesForMachine returns lxd profile names mapped to the
// lxd profile they represent for applications on the given machine.
func (st *State) GetLXDProfilesForMachine(ctx context.Context, mName string) ([]internal.CreateLXDProfileDetails, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lxdProfileStmt, err := st.Prepare(`
SELECT (c.lxd_profile, c.revision, a.name) AS (&lxdProfileAndName.*)
FROM   charm AS c
JOIN   application AS a ON c.uuid = a.charm_uuid
JOIN   unit AS u ON a.uuid = u.application_uuid
JOIN   machine AS m ON u.net_node_uuid = m.net_node_uuid
WHERE  m.name = $entityName.name
AND    c.lxd_profile IS NOT NULL AND c.lxd_profile != ''
`, entityName{}, lxdProfileAndName{})
	if err != nil {
		return nil, errors.Errorf("preparing query of lxd profiles for machine %s: %w", mName, err)
	}

	var profiles []lxdProfileAndName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lxdProfileStmt, entityName{Name: mName}).GetAll(&profiles)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("querying lxd profiles for machine %s: %w", mName, err)
	}

	result := make([]internal.CreateLXDProfileDetails, len(profiles))
	for i, profile := range profiles {
		result[i] = internal.CreateLXDProfileDetails{
			ApplicationName: profile.AppName,
			CharmRevision:   profile.Revision,
			LXDProfile:      profile.LXDProfile,
		}
	}

	return result, nil
}
