// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/cmd/v3"

	"github.com/juju/juju/cmd/jujud/introspect"
)

func main() {
	docsDir := mustEnv("DOCS_DIR")
	err := os.MkdirAll(docsDir, 0777)
	check(err)

	docPath := filepath.Join(docsDir, "juju-introspect.md")
	file, err := os.Create(docPath)
	check(err)

	err = cmd.PrintMarkdown(file, &introspect.IntrospectCommand{}, cmd.MarkdownOptions{})
	check(err)

	err = file.Close()
	check(err)
}

// UTILITY FUNCTIONS

// check panics if the provided error is not nil.
func check(err error) {
	if err != nil {
		panic(err)
	}
}

// Returns the value of the given environment variable, panicking if the var
// is not set.
func mustEnv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		panic(fmt.Sprintf("env var %q not set", key))
	}
	return val
}
