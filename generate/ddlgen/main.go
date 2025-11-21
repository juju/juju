// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// ddlgen is a tool used to generate the DDLs for the model and controller
// databases for every patch version of this major.minor (with the exception of
// the current version i.e. the in development version).
//
// We use this to verify that DDLs do not change in invalid ways.
//
// We need this because, to support controller upgrades, the only changes we can make
// to the DDL between patch releases are additive. Applying the DDL is idempotent
// using stored hashes of the patches we've already applied. These hashes are
// calculated before the DDL is parsed, meaning comments, whitespace, etc. also
// cannot change.

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/juju/collections/transform"

	coreschema "github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/schema"
)

func main() {
	current := version.Current
	if current.Patch == 0 {
		// Nothing to generate
		return
	}

	version := current
	for p := 0; p < current.Patch; p++ {
		version.Patch = p
		writeModelDDL(version)
		writeControllerDDL(version)
	}
}

const (
	controllerDDLFileForVersion = "domain/schema/controller/%v-controller-release.ddl"
	modelDDLFileForVersion      = "domain/schema/model/%v-model-release.ddl"
)

const header = `-- Code genrated by ddlgen. DO NOT EDIT.
-- Source: github.com/juju/juju/generate/ddlgen

`

func writeControllerDDL(version semversion.Number) {
	filename := fmt.Sprintf(controllerDDLFileForVersion, version)

	controllerDDL := schema.ControllerDDLForPatchVersion(version.Patch).Patches()
	formattedDDL := strings.Join(transform.Slice(controllerDDL, func(p coreschema.Patch) string { return coreschema.Stmt(p) }), "\n")

	ofh, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := ofh.Close(); err != nil {
			panic(err)
		}
	}()

	if _, err := ofh.WriteString(header); err != nil {
		panic(err)
	}

	if _, err := ofh.WriteString(formattedDDL); err != nil {
		panic(err)
	}
}

func writeModelDDL(version semversion.Number) {
	filename := fmt.Sprintf(modelDDLFileForVersion, version)

	modelDDL := schema.ModelDDLForPatchVersion(version.Patch).Patches()
	formattedDDL := strings.Join(transform.Slice(modelDDL, func(p coreschema.Patch) string { return coreschema.Stmt(p) }), "\n")

	ofh, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := ofh.Close(); err != nil {
			panic(err)
		}
	}()

	if _, err := ofh.WriteString(header); err != nil {
		panic(err)
	}

	if _, err := ofh.WriteString(formattedDDL); err != nil {
		panic(err)
	}
}
