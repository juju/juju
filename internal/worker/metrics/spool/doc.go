// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package spool contains the implementation of a
// worker that extracts the spool directory path from the agent
// config and enables other workers to write and read
// metrics to and from a the spool directory using a writer
// and a reader.
package spool
