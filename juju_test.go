// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

var (
	_ EntityInfo = (*MachineInfo)(nil)
	_ EntityInfo = (*ServiceInfo)(nil)
	_ EntityInfo = (*UnitInfo)(nil)
	_ EntityInfo = (*RelationInfo)(nil)
	_ EntityInfo = (*AnnotationInfo)(nil)
)
