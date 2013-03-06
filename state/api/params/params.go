package params

// ServiceExpose holds the parameters for making the ServiceExpose call.
type ServiceExpose struct {
	ServiceName string
}

// ServiceSet holds the parameters for a ServiceSet
// command. Options contains the configuration data.
type ServiceSet struct {
	ServiceName string
	Options     map[string]string
}

// ServiceSetYAML holds the parameters for
// a ServiceSetYAML command. Config contains the
// configuration data in YAML format.
type ServiceSetYAML struct {
	ServiceName string
	Config      string
}

// ServiceGet holds parameters for making the ServiceGet call.
type ServiceGet struct {
	ServiceName string
}

// ServiceGetResults holds results of the ServiceGet call.
type ServiceGetResults struct {
	Service  string
	Charm    string
	Settings map[string]interface{}
}

// ServiceUnexpose holds parameters for the ServiceUnexpose call.
type ServiceUnexpose struct {
	ServiceName string
}

// Creds holds credentials for identifying an entity.
type Creds struct {
	EntityName string
	Password   string
}

// Machine holds details of a machine.
type Machine struct {
	InstanceId string
}

// EntityWatcherId holds the id of an EntityWatcher.
type EntityWatcherId struct {
	EntityWatcherId string
}

// Password holds a password.
type Password struct {
	Password string
}

// Unit holds details of a unit.
type Unit struct {
	DeployerName string
	// TODO(rog) other unit attributes.
}

// User holds details of a user.
type User struct {
	// This is a placeholder for any information
	// that may be associated with a user in the
	// future.
}

// GetAnnotations stores parameters for making the GetAnnotations call.
type GetAnnotations struct {
	Id string
}

// SetAnnotation stores parameters for making the SetAnnotation call.
type SetAnnotation struct {
	Id    string
	Key   string
	Value string
}
