// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package lxdprofile defines a set of functions and constants that can
// interact with LXD Profiles. LXD Profiles are key/value YAML configuration
// files that the LXD provider and LXD container broker can consume and apply
// to a container.
//
// More information about a type of LXD configuration profile can found
// https://github.com/canonical/lxd/blob/master/doc/containers.md
//
// LXDProfile package defines core concepts that can be utilised from different
// packages of the codebase, that want to work with a LXD profile. Not all
// key/value configurations from the underlying LXD can be applied and some
// key/values need to be applied using `--force`. To validate the LXD Profile
// before attempting to apply it to a ubuntu LXD container, there are some
// validation functions.
//
// Each LXDProfile is given a unique name when applied to the container. This
// is for three reasons:
//  1. readability - when an operator is attempting to debug if a LXD Profile
//     has been applied for a given charm revision, it should be straightforward
//     for the operator to read the output of `lxd profile list` to marry them.
//  2. Collisions - to ensure that no other charm profile can't collide with in
//     the LXC namespace for an existing LXD Profile, each profile
//     namespaces in the following way `juju-<model>-<application>-<charm-revision>`
//  3. Removability - juju needs to clean up LXD profiles that where applied to
//     the LXD container, but are subsequently not required any more, either by
//     an upgrade of the charm or the complete removal of the charm. Either way,
//     the removal of the charms from the LXC profile list is to prevent orphan
//     profiles from being left dangling.
package lxdprofile
