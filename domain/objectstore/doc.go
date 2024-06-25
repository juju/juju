// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package objectstore provides a service to keep track of metadata about
// objects (binary blobs) that are stored in juju's object store. These objects
// can be any large blob of data that juju needs to store, for example, a
// downloaded charm.

package objectstore
