// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

/*
utils/filestorage provides types for abstracting and implementing a
system that stores files, including their metadata.

Each file in the system is identified by a unique ID, determined by the
system at the time the file is stored.

File metadata includes such information as the size of the file, its
checksum, and when it was created.  Regardless of how it is stored in
the system, at the abstraction level it is represented as a document.

Metadata can exist in the system without an associated file.  However,
every file must have a corresponding metadata doc stored in the system.
A file can be added for a metadata doc that does not have one already.

The main type is the FileStorage interface.  It exposes the core
functionality of such a system.  This includes adding/removing files,
retrieving them or their metadata, and listing all files in the system.

The package also provides a basic implementation of FileStorage,
available through NewFileStorage().  This implementation simply wraps
two more focused systems: doc storage and raw file storage.  The wrapper
uses the doc storage to store the metadata and raw file storage to
store the files.

The two subsystems are exposed via corresponding interfaces: DocStorage
(and its specialization MetadataStorage) and RawFileStorage.  While a
single type could implement both, in practice they will be separate.
The doc storage is responsible to generating the unique IDs.  The raw
file storage defers to the doc storage for any information about the
file, including the ID.

*/
package filestorage
