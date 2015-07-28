package components

// RegisterUnitStatusFormatter registers a function that returns a
// value that will be used to deserialize the status value for the given
// component.
func RegisterUnitStatusFormatter(component string, fn func([]byte) interface{}) {
	UnitStatusFormatters[component] = fn
}

var UnitStatusFormatters = map[string]func([]byte) interface{}{}
