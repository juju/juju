// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package life defines the life of Juju entities.
//
// The life of an entity is one of three values, each a [Value]:
//
//   - Alive indicates that the entity is meant to exist;
//   - Dying indicates that the entity should be removed;
//   - Dead indicates that the entity is no longer useful and can be
//     destroyed unconditionally.
//
// Value.Validate reports whether a value is one of these three, and the
// predicates IsAlive, IsNotAlive, IsDead and IsNotDead test a value: an
// entity that is not alive is at some stage of destruction or cleanup, and
// an entity that is not dead is active in some way.
//
// Entities that have a life include machines, units, applications and
// relations. See github.com/juju/juju/domain/machine,
// github.com/juju/juju/domain/application (which also persists the life of
// units) and github.com/juju/juju/domain/relation, and
// github.com/juju/juju/domain/life for the integer form recorded in the life
// lookup table.
package life
