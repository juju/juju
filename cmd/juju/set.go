package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/log"
)

// SetCommand updates the configuration of a service
type SetCommand struct {
	EnvName     string
	ServiceName string
	// either Options or Config will contain the configuration data
	Options []string
	Config  cmd.FileVar
}

func (c *SetCommand) Info() *cmd.Info {
	return &cmd.Info{"set", "", "set service config options", ""}
}

func (c *SetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.Var(&c.Config, "config", "path to yaml-formatted service config")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	if len(args) == 0 || len(strings.Split(args[0], "=")) > 1 {
		return errors.New("no service name specified")
	}
	c.ServiceName, c.Options = args[0], args[1:]
	return nil
}

// Run updates the configuration of a service
func (c *SetCommand) Run(ctx *cmd.Context) error {
	contents, err := c.Config.Read(ctx)
	if err != nil && err != cmd.PathNotSetError {
		return err
	}
	var (
		unvalidated = make(map[string]string)
		remove      []string
	)
	if len(contents) > 0 {
		if err := goyaml.Unmarshal(contents, &unvalidated); err != nil {
			return err
		}
	}
	if len(unvalidated) == 0 {
		unvalidated, remove, err = parse(c.Options)
		if err != nil {
			return err
		}
	}
	conn, err := juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	srv, err := conn.State.Service(c.ServiceName)
	if err != nil {
		return err
	}
	charm, _, err := srv.Charm()
	if err != nil {
		return err
	}
	chcfg := charm.Config()
	// 1. Validate will convert this partial configuration
	// into a full configuration by inserting charm defaults
	// for missing values.
	validated, err := chcfg.Validate(unvalidated)
	if err != nil {
		return err
	}
	// 2. strip out the additional default keys added in the previous step.
	validated = strip(validated, chcfg)
	cfg, err := srv.Config()
	if err != nil {
		return err
	}

	// 3. Update any keys that remain after validation and filtering.
	if len(validated) > 0 {
		log.Debugf("updating configuration items: %v", validated)
		cfg.Update(validated)
	}

	// 4. Delete any removed keys.
	if len(remove) > 0 {
		log.Debugf("removing configuration items: %v", remove)
		for _, k := range remove {
			cfg.Delete(k)
		}
	}
	_, err = cfg.Write()
	return err
}

// parse parses the option k=v strings into a map of options to be 
// updated in the config. Keys with empty values are returned separately
// and should be removed.
func parse(options []string) (map[string]string, []string, error) {
	var (
		m = make(map[string]string)
		d []string
	)
	for _, o := range options {
		s := strings.Split(o, "=")
		if len(s) != 2 {
			return nil, nil, fmt.Errorf("invalid option: %q", o)
		}
		if len(s[1]) > 0 {
			m[s[0]] = s[1]
		} else {
			d = append(d, s[0])
		}
	}
	return m, d, nil
}

// strip removes from options any keys whos values match the charm defaults.
func strip(options map[string]interface{}, config *charm.Config) map[string]interface{} {
	for k, v := range options {
		if ch, ok := config.Options[k]; ok {
			if reflect.DeepEqual(ch.Default, v) {
				delete(options, k)
			}
		}
	}
	return options
}
