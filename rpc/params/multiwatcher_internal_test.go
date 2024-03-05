// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

var (
	_ EntityInfo = (*MachineInfo)(nil)
	_ EntityInfo = (*ApplicationInfo)(nil)
	_ EntityInfo = (*CharmInfo)(nil)
	_ EntityInfo = (*RemoteApplicationUpdate)(nil)
	_ EntityInfo = (*ApplicationOfferInfo)(nil)
	_ EntityInfo = (*UnitInfo)(nil)
	_ EntityInfo = (*RelationInfo)(nil)
	_ EntityInfo = (*BlockInfo)(nil)
	_ EntityInfo = (*ActionInfo)(nil)
	_ EntityInfo = (*ModelUpdate)(nil)
	_ EntityInfo = (*BranchInfo)(nil)
)
