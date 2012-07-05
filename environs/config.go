package environs

import (
	"errors"
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"os"
	"path/filepath"
)

// environ holds information about one environment.
type environ struct {
	config EnvironConfig
	err    error // an error if the config data could not be parsed.
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

// RegisterProvider registers a new environment provider. Name gives the name
// of the provider, and p the interface to that provider.
//
// RegisterProvider will panic if the same provider name is registered more than
// once.
func RegisterProvider(name string, p EnvironProvider) {
	if providers[name] != nil {
		panic(fmt.Errorf("juju: duplicate provider name %q", name))
	}
	providers[name] = p
}

// ReadEnvironsBytes parses the contents of an environments.yaml file
// and returns its representation. An environment with an unknown type
// will only generate an error when New is called for that environment.
// Attributes for environments with known types are checked.
func ReadEnvironsBytes(data []byte) (*Environs, error) {
	var raw struct {
		Default      string
		Environments map[string]map[string]interface{}
	}
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
	badEnv := func(name string, err error) {
		environs[name] = environ{err: err}
	}

	for name, attrs := range raw.Environments {
		kind, _ := attrs["type"].(string)
		if kind == "" {
			badEnv(name, fmt.Errorf("environment %q has no type", name))
			continue
		}
		p := providers[kind]
		if p == nil {
			badEnv(name, fmt.Errorf("environment %q has an unknown provider type %q", name, kind))
			continue
		}
		// store the name of the this environment in the config itself
		// so that providers can see it.
		attrs["name"] = name
		cfg, err := p.NewConfig(attrs)
		if err != nil {
			badEnv(name, fmt.Errorf("error parsing environment %q: %v", name, err))
			continue
		}
		environs[name] = environ{config: cfg}
	}
	return &Environs{raw.Default, environs}, nil
}

// ReadEnvirons reads the juju environments.yaml file
// and returns the result of running ParseEnvironments
// on the file's contents.
// If environsFile is empty, $HOME/.juju/environments.yaml
// is used.
func ReadEnvirons(environsFile string) (*Environs, error) {
	if environsFile == "" {
		home := os.Getenv("HOME")
		if home == "" {
			return nil, errors.New("$HOME not set")
		}
		environsFile = filepath.Join(home, ".juju/environments.yaml")
	}
	data, err := ioutil.ReadFile(environsFile)
	if err != nil {
		return nil, err
	}
	e, err := ReadEnvironsBytes(data)
	if err != nil {
		return nil, fmt.Errorf("cannot parse %q: %v", environsFile, err)
	}
	return e, nil
}
