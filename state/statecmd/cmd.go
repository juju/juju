package statecmd

type SetParams struct {
	ServiceName string
	Options map[string]string
	// either Options or Config will contain the configuration data
	Config string
}

func SetConfig(st *state.State, p SetParams) error {
	var unvalidated = make(map[string]string)
	var remove []string
	if len(p.Config) > 0 {
		if err := goyaml.Unmarshal([]byte(p.Contents), &unvalidated); err != nil {
			return err
		}
	} else if len(c.Options) == 0 {
		return errors.New("no options to set")
	}
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
		log.Debugf("cmd/juju: updating configuration items: %v", validated)
		cfg.Update(validated)
	}
	// 4. Delete any removed keys.
	if len(remove) > 0 {
		log.Debugf("cmd/juju: removing configuration items: %v", remove)
		for _, k := range remove {
			cfg.Delete(k)
		}
	}
	_, err = cfg.Write()
	return err
	