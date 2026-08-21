// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

// ScriptletApplication is the scalar persistence representation of a
// scriptlet-backed application.
type ScriptletApplication struct {
	UUID    string
	Name    string
	Life    int
	Sources []ScriptSource
}

// ScriptSource is the scalar persistence representation of a Starform source.
type ScriptSource struct {
	LoadPath string
	Source   string
}
