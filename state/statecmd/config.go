package statecmd
import (
	"errors"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/log"
)

// SetConfigParams holds the parameters for a SetConfig
// command. Either Options or Config will contain the configuration data.
type SetConfigParams struct {
	ServiceName string
	Options map[string]string
	Config string
}

// SetConfig changes a service's configuration values.
// Values set to the empty string will be deleted.
func SetConfig(st *state.State, p SetConfigParams) error {
	var options map[string] string
	if len(p.Config) > 0 {
		if err := goyaml.Unmarshal([]byte(p.Config), &options); err != nil {
			return err
		}
	} else {
		options = p.Options
	}
	if len(options) == 0 {
		return errors.New("no options to set")
	}
	unvalidated := make(map[string]string)
	var remove []string
	for k, v := range options {
		if v == "" {
			remove = append(remove, k)
		} else {
			unvalidated[k] = v
		}
	}
	log.Printf("remove: %q; unvalidated: %q", remove, unvalidated)
	srv, err := st.Service(p.ServiceName)
	if err != nil {
		return err
	}
	charm, _, err := srv.Charm()
	if err != nil {
		return err
	}
	// 1. Validate will convert this partial configuration
	// into a full configuration by inserting charm defaults
	// for missing values.
	validated, err := charm.Config().Validate(unvalidated)
	if err != nil {
		return err
	}
	// 2. strip out the additional default keys added in the previous step.
	validated = strip(validated, unvalidated)
	cfg, err := srv.Config()
	if err != nil {
		return err
	}
	// 3. Update any keys that remain after validation and filtering.
	if len(validated) > 0 {
		log.Debugf("state/statecmd: updating configuration items: %v", validated)
		cfg.Update(validated)
	}
	// 4. Delete any removed keys.
	if len(remove) > 0 {
		log.Debugf("state/statecmd: removing configuration items: %v", remove)
		for _, k := range remove {
			cfg.Delete(k)
		}
	}
	_, err = cfg.Write()
	return err
}

// strip removes from validated, any keys which are not also present in unvalidated.
func strip(validated map[string]interface{}, unvalidated map[string]string) map[string]interface{} {
	for k := range validated {
		if _, ok := unvalidated[k]; !ok {
			delete(validated, k)
		}
	}
	return validated
}
