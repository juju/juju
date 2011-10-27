package juju

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"launchpad.net/goyaml"
)

// environ holds information about one environment.
type environ struct {
	kind   string      // the type of environment (e.g. ec2).
	config interface{} // the configuration data for passing to NewEnviron.
	err    os.Error    // an error if the config data could not be parsed.
}

// Environs holds information about each named environment
// in an environments.yaml file.
type Environs struct {
	Default  string // The name of the default environment.
	environs map[string]environ
}

// Names returns the list of environment names.
func (e *Environs) Names() (names []string) {
	for name := range e.environs {
		names = append(names, name)
	}
	return
}

// providers maps from provider type to EnvironProvider for
// each registered provider type.
var providers = make(map[string]EnvironProvider)

// Register registers a new environment provider. Name gives the name
// of the provider, and p the interface to that provider.
// If p is nil, the name will be unregistered.
//
// Register will panic if the same provider name is registered more than
// once.
func Register(name string, p EnvironProvider) {
	if providers[name] != nil && p != nil {
		panic(fmt.Errorf("juju: duplicate provider name %q", name))
	}
	providers[name] = p
}

// ParseEnvironments parses the YAML data
// and returns an Environs value representing
// the environments specified in the file.
// An environments with an unknown type will only
// generate an error when New is called for that environment.
// Attributes for environments with known types
// are checked.
func ParseEnvironments(data []byte) (*Environs, os.Error) {
	var raw struct {
		Default      string                 "default"
		Environments map[string]interface{} "environments"
	}
	raw.Environments = make(map[string]interface{}) // TODO fix bug in goyaml
	err := goyaml.Unmarshal(data, &raw)
	if err != nil {
		return nil, err
	}

	if raw.Default != "" && raw.Environments[raw.Default] == nil {
		return nil, fmt.Errorf("default environment %q does not exist", raw.Default)
	}
	if raw.Default == "" {
		// If there's a single environment, then we get the default
		// automatically.
		if len(raw.Environments) == 1 {
			for name := range raw.Environments {
				raw.Default = name
				break
			}
		}
	}

	environs := make(map[string]environ)
	for name, x := range raw.Environments {
		attrs, ok := x.(map[interface{}]interface{})
		if !ok {
			return nil, fmt.Errorf("environment %q does not have attributes", name)
		}
		kind, _ := attrs["type"].(string)
		if kind == "" {
			return nil, fmt.Errorf("environment %q has no type", name)
		}

		p := providers[kind]
		if p == nil {
			// unknown provider type - skip entry but leave error message
			// in case the environment is used later.
			environs[name] = environ{
				kind: kind,
				err:  fmt.Errorf("environment %q has an unknown provider type: %q", name, kind),
			}
			continue
		}
		cfg, err := p.ConfigChecker().Coerce(attrs, nil) // TODO sensible value for path?
		if err != nil {
			return nil, fmt.Errorf("error parsing environment %q: %v", name, err)
		}
		environs[name] = environ{
			kind:   kind,
			config: cfg,
		}
	}
	return &Environs{raw.Default, environs}, nil
}

// ReadEnvironments reads the juju environments.yaml file
// and returns the result of running ParseEnvironments
// on the file's contents.
// If environmentsFile is empty, $HOME/.juju/environments.yaml
// is used.
func ReadEnvironments(environmentsFile string) (*Environs, os.Error) {
	if environmentsFile == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return nil, os.NewError("$HOME not set")
		}
		environmentsFile = filepath.Join(home, ".juju/environments.yaml")
	}
	data, err := ioutil.ReadFile(environmentsFile)
	if err != nil {

		return nil, err
	}
	e, err := ParseEnvironments(data)
	if err != nil {
		fmt.Errorf("cannot parse %q: %v", environmentsFile, err)
	}
	return e, nil
}
