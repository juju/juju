// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "maps"

// Attrs is a convenience type for messing
// around with configuration attributes.
type Attrs map[string]any

func (a Attrs) Merge(with Attrs) Attrs {
	new := make(Attrs)
	maps.Copy(new, a)
	maps.Copy(new, with)
	return new
}

func (a Attrs) Delete(attrNames ...string) Attrs {
	new := make(Attrs)
	maps.Copy(new, a)
	for _, attr := range attrNames {
		delete(new, attr)
	}
	return new
}
