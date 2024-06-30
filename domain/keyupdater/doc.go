// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// keyupdater package provides the domain knowledge for retrieving the
// authorised keys for the different entities within Juju. Specifically machines
// can ask what authorised keys they should have on the system.
//
// This domain uncouples the caller from having to understand all the different
// sources of authroised keys within Juju.
package keyupdater
