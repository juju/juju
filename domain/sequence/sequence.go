// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sequence

import "fmt"

// Namespace represents a namespace for a sequence number.
type Namespace interface {
	fmt.Stringer
}

// StaticNamespace is a static namespace for a sequence number. There are
// no dynamic parts to the namespace.
type StaticNamespace string

func (n StaticNamespace) String() string {
	return string(n)
}

// PrefixNamespace is a dynamic namespace for a sequence number. The
// namespace is generated from a string and a sequence number.
type PrefixNamespace struct {
	Prefix Namespace
	name   string
}

// MakePrefixNamespace creates a new PrefixNamespace with the given prefix and
// name.
func MakePrefixNamespace(prefix Namespace, suffix string) PrefixNamespace {
	return PrefixNamespace{
		Prefix: prefix,
		name:   suffix,
	}
}

func (n PrefixNamespace) String() string {
	return fmt.Sprintf("%s_%s", n.Prefix, n.name)
}
