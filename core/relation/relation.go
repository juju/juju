// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

// JujuInfo is the name and interface of an implicit relation, with role
// requires, added to every charm. It facilitates integrations between
// subordinate and other charms which otherwise may not have a common
// interface.
const JujuInfo = "juju-info"
