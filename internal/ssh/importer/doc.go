// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Importer package provides a means for importing public ssh keys from a well
// known internet based location such as Github or Launchpad.
//
// [NewImporter] can be used to construct a new ssh public key importer.
// Currently Github (gh) and Launchpad (lp) are supported for importing public
// keys.
//
// Examples of import subjects:
// - gh:tlm
// - gh:~wallyworld
package importer
