// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The description package defines the structure and representation and
// serialisation of models to facilitate the import and export of
// models from different controllers.
package description

// NOTES:
//
// The following prechecks are to be made before attempting migration:
//
// - no agents in an error state
// - nothing dying or dead; machine, service, unit, relation, storage, network etc
// - no entries in the assignUnitC collection
//   - these are units pending assignment
// - no units agent status in an error state
//   - workload error status is probably fine
// - all units using the same charm and series as the service
//   - no units with pending charm updates
// - all units have ResolvedNone for resolved status
//   - no pending hook execution
