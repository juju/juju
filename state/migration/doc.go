// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The migration package defines the structure and representation and
// serialisation of model descriptions to facilitate the import and export of
// models from different controllers.
//
// Conceptually there are three levels of managing the migration.
//
// 1) There is the structural represenation of the Model and all the
//    associated components: machines, services, units, storage and networks.
// 2) There is an abstraction above the model representation that includes
//    a version number, and a way to represent the binary blobs that are
//    tools and charms, and later, resources.
// 3) There is a way to turn the abstraction from (2) above into a stream,
//    and also from a stream.
//
// The current design ideas is to be able to turn (2) into a zip file that
// contains a YAML serialised model description, version, and a collection
// of binary files.
package migration
