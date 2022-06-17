// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package output

import "github.com/juju/ansiterm"

// Colors holds Color for each of the JSON and YAML data types.
type Colors struct {
	// Null is the Color for JSON nil.
	Null *ansiterm.Context
	// Bool is the Color for boolean values.
	Bool *ansiterm.Context
	// Binary is the color used for yaml binary data type.
	Binary *ansiterm.Context
	// Number is the Color for number values.
	Number *ansiterm.Context
	// String is the Color for string values.
	String *ansiterm.Context
	// Key is the Color for JSON keys.
	Key *ansiterm.Context
	// KeyValSep separates key from values.
	KeyValSep *ansiterm.Context
	// DocumentStart is the color used to mark the beginning of a valid yaml document
	DocumentStart *ansiterm.Context
	// Comment is the color used for yaml comments
	Comment *ansiterm.Context
	// Ip is the color for ip addresses
	Ip *ansiterm.Context
	// Scalar used to color scalar values
	Scalar *ansiterm.Context
	// IndentLine used to color indentation lines
	IndentLine *ansiterm.Context
	// Dash colors the beginning of a yaml item list
	Dash *ansiterm.Context
	// EmptyStructure colors the empty open and closing brackets '[]'
	EmptyStructure *ansiterm.Context
	// NodeAnchor colors a yaml node anchor definition.
	NodeAnchor *ansiterm.Context
}
