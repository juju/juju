// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package elfnote is a build utility used by the Makefile to embed
// the juju version in a ELF file as a note section. It is used
// in development only.
// The package github.com/juju/juju/internal/devtools reads this
// note section for local dev builds.
// The format of the ELF note section is standard. The description
// stores the version string.
package main
