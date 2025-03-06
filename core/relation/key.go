// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package relation

// Key is the natural key of a relation. "application:endpoint application:endpoint"
// in sorted order based on the application.
type Key string

func (k Key) String() string {
	return string(k)
}
