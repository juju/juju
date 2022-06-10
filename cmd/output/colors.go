package output

import "github.com/juju/ansiterm"

// Colors holds Color for each of the JSON and YAML data types.
type Colors struct {
	// Null is the Color for JSON nil.
	Null *ansiterm.Context
	// Bool is the Color for boolean values.
	Bool *ansiterm.Context
	// Number is the Color for number values.
	Number *ansiterm.Context
	// String is the Color for string values.
	String *ansiterm.Context
	// Key is the Color for JSON keys.
	Key *ansiterm.Context
	//KeyValSep separates key from values.
	KeyValSep *ansiterm.Context
}
