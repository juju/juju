package testing

// Attrs is a convenience type for messing
// around with configuration attributes.
type Attrs map[string]interface{}

func (a Attrs) Merge(with Attrs) Attrs {
	new := make(Attrs)
	for attr, val := range a {
		new[attr] = val
	}
	for attr, val := range with {
		new[attr] = val
	}
	return new
}

func (a Attrs) Delete(attrNames ...string) Attrs {
	new := make(Attrs)
	for attr, val := range a {
		new[attr] = val
	}
	for _, attr := range attrNames {
		delete(new, attr)
	}
	return new
}
