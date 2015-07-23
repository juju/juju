package components

// RegisterUnitComponentFormatter registers a formatting function for the given component.  When status is returned from the API, unitstatus for the compoentn
func RegisterUnitComponentFormatter(component string, fn func(apiobj interface{}) interface{}) {
	UnitComponentFormatters[component] = fn
}

var UnitComponentFormatters = map[string]func(interface{}) interface{}{}
