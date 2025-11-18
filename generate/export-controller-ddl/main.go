// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"strings"

	"github.com/juju/collections/transform"

	coreschema "github.com/juju/juju/core/database/schema"
	"github.com/juju/juju/domain/schema"
)

func main() {
	controllerDDL := schema.ControllerDDLWithoutPatches().Patches()
	formattedDDL := strings.Join(transform.Slice(controllerDDL, func(p coreschema.Patch) string { return coreschema.Stmt(p) }), "\n")
	fmt.Print(formattedDDL)
}
