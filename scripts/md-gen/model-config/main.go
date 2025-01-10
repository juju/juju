// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/environschema"
)

// Generate Markdown documentation based on the contents of the
// github.com/juju/juju/environs/config package.
func main() {
	// Gather information from model config schema.
	data := fillFromSchema()

	// Print generated docs.
	fmt.Print(render(data))
}

// keyInfo contains information about a config key.
type keyInfo struct {
	Key          string // e.g. "agent-ratelimit-max"
	ConstantName string // e.g. "AgentRateLimitMax"
	Type         string
	Doc          string // from parsing comments in config.go
	Immutable    bool   // from AllowedUpdateConfigAttributes
	Mandatory    bool
	Deprecated   bool
	Default      string // from instantiating NewConfig

	SetByJuju   bool
	ValidValues []string
}

// render turns the input data into a Markdown document
func render(data map[string]*keyInfo) string {
	var mainDoc string

	anchorForKey := func(key string) string {
		return key
	}
	headingForKey := func(key string) string {
		return "## " + anchorForKey(key)
	}

	// Sort keys
	var keys []string
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		info := data[key]

		mainDoc += headingForKey(key) + "\n"
		if info.Deprecated {
			mainDoc += "> This key is deprecated.\n"
		}
		mainDoc += "\n"

		if info.SetByJuju {
			mainDoc += "*Note: This value is set by Juju.*\n\n"
		} else {
			// Only print these if the value can be set by a user, otherwise it is of no use.
			if info.Immutable {
				mainDoc += "*Note: This value cannot be changed after model creation.* \n\n"
			}
			if info.Mandatory {
				mainDoc += "*Note: This value must be set.* \n\n"
			}
		}

		// Ensure doc has fullstop/newlines at end
		mainDoc += strings.TrimRight(info.Doc, ".\n") + ".\n\n"

		// Always print the default value
		mainDoc += "**Default value:** " + info.Default + "\n\n"

		if info.Type != "" {
			mainDoc += "**Type:** " + info.Type + "\n\n"
		}
		if len(info.ValidValues) > 0 {
			mainDoc += "**Valid values:** " + strings.Join(info.ValidValues, ", ") + "\n\n"
		}

		mainDoc += "\n"
	}

	return mainDoc
}

// Get data from config.Schema.
func fillFromSchema() map[string]*keyInfo {
	data := map[string]*keyInfo{}
	schema, err := config.Schema(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: getting model config schema: %s", err)
		os.Exit(1)
	}

	defaults := config.ConfigDefaults()

	for key, attr := range schema {
		if data[key] == nil {
			data[key] = &keyInfo{
				Key: key,
			}
		}

		if attr.Group == environschema.JujuGroup {
			data[key].SetByJuju = true
		}

		data[key].Doc = attr.Description
		data[key].Type = string(attr.Type)
		data[key].Immutable = attr.Immutable
		data[key].Mandatory = attr.Mandatory
		if d, ok := defaults[key]; ok {
			data[key].Default = fmt.Sprint(d)
		}

		for _, val := range attr.Values {
			data[key].ValidValues = append(data[key].ValidValues, fmt.Sprint(val))
		}
	}

	return data
}
