package juju

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/trivial"
	"net/url"
	"os"
	"strings"
	"time"
)

// Conn holds a connection to a juju environment and its
// associated state.
type Conn struct {
	Environ environs.Environ
	State   *state.State
}

var redialStrategy = trivial.AttemptStrategy{
	Total: 60 * time.Second,
	Delay: 250 * time.Millisecond,
}

// NewConn returns a new Conn that uses the
// given environment. The environment must have already
// been bootstrapped.
func NewConn(environ environs.Environ) (*Conn, error) {
	info, _, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	password := environ.Config().AdminSecret()
	if password == "" {
		return nil, fmt.Errorf("cannot connect without admin-secret")
	}
	info.Password = password
	st, err := state.Open(info)
	if state.IsUnauthorizedError(err) {
		// We can't connect with the administrator password,;
		// perhaps this was the first connection and the
		// password has not been changed yet.
		info.Password = trivial.PasswordHash(password)

		// We try for a while because we might succeed in
		// connecting to mongo before the state has been
		// initialized and the initial password set.
		for a := redialStrategy.Start(); a.Next(); {
			st, err = state.Open(info)
			if !state.IsUnauthorizedError(err) {
				break
			}
		}
		if err != nil {
			return nil, err
		}
		if err := st.SetAdminMongoPassword(password); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	conn := &Conn{
		Environ: environ,
		State:   st,
	}
	if err := conn.updateSecrets(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("unable to push secrets: %v", err)
	}
	return conn, nil
}

// NewConnFromState returns a Conn that uses an Environ
// made by reading the environment configuration.
// The resulting Conn uses the given State - closing
// it will close that State.
func NewConnFromState(st *state.State) (*Conn, error) {
	cfg, err := st.EnvironConfig()
	if err != nil {
		return nil, err
	}
	environ, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	return &Conn{
		Environ: environ,
		State:   st,
	}, nil
}

// NewConnFromName returns a Conn pointing at the environName environment, or the
// default environment if not specified.
func NewConnFromName(environName string) (*Conn, error) {
	environ, err := environs.NewFromName(environName)
	if err != nil {
		return nil, err
	}
	return NewConn(environ)
}

// Close terminates the connection to the environment and releases
// any associated resources.
func (c *Conn) Close() error {
	return c.State.Close()
}

// updateSecrets writes secrets into the environment when there are none.
// This is done because environments such as ec2 offer no way to securely
// deliver the secrets onto the machine, so the bootstrap is done with the
// whole environment configuration but without secrets, and then secrets
// are delivered on the first communication with the running environment.
func (c *Conn) updateSecrets() error {
	secrets, err := c.Environ.Provider().SecretAttrs(c.Environ.Config())
	if err != nil {
		return err
	}
	cfg, err := c.State.EnvironConfig()
	if err != nil {
		return err
	}
	attrs := cfg.AllAttrs()
	for k := range secrets {
		if _, exists := attrs[k]; exists {
			// Environment already has secrets. Won't send again.
			return nil
		}
	}
	cfg, err = cfg.Apply(secrets)
	if err != nil {
		return err
	}
	return c.State.SetEnvironConfig(cfg)
}

// PutCharm uploads the given charm to provider storage, and adds a
// state.Charm to the state.  The charm is not uploaded if a charm with
// the same URL already exists in the state.
// If bumpRevision is true, the charm must be a local directory,
// and the revision number will be incremented before pushing.
func (conn *Conn) PutCharm(curl *charm.URL, repo charm.Repository, bumpRevision bool) (*state.Charm, error) {
	if curl.Revision == -1 {
		rev, err := repo.Latest(curl)
		if err != nil {
			return nil, fmt.Errorf("cannot get latest charm revision: %v", err)
		}
		curl = curl.WithRevision(rev)
	}
	ch, err := repo.Get(curl)
	if err != nil {
		return nil, fmt.Errorf("cannot get charm: %v", err)
	}
	if bumpRevision {
		chd, ok := ch.(*charm.Dir)
		if !ok {
			return nil, fmt.Errorf("cannot increment revision of charm %q: not a directory", curl)
		}
		if err = chd.SetDiskRevision(chd.Revision() + 1); err != nil {
			return nil, fmt.Errorf("cannot increment revision of charm %q: %v", curl, err)
		}
		curl = curl.WithRevision(chd.Revision())
	}
	if sch, err := conn.State.Charm(curl); err == nil {
		return sch, nil
	}
	return conn.addCharm(curl, ch)
}

// DeployServiceParams contains the arguments required to deploy the referenced
// charm.
type DeployServiceParams struct {
	Charm       *state.Charm
	ServiceName string
	NumUnits    int
	// Config is used only by the API.
	Config map[string]string
	// ConfigYAML takes precedence over Config if both are provided.
	ConfigYAML string
}

// DeployService takes a charm and various parameters and deploys it.
func (conn *Conn) DeployService(args DeployServiceParams) (*state.Service, error) {

	svc, err := conn.State.AddService(args.ServiceName, args.Charm)
	if err != nil {
		return nil, err
	}

	if args.ConfigYAML != "" {
		ssArgs := params.ServiceSetYAML{
			ServiceName: args.ServiceName,
			Config:      args.ConfigYAML,
		}
		if err := ServiceSetYAML(conn.State, ssArgs); err != nil {
			return nil, err
		}
	} else if args.Config != nil {
		ssArgs := params.ServiceSet{
			ServiceName: args.ServiceName,
			Options:     args.Config,
		}
		if err := ServiceSet(conn.State, ssArgs); err != nil {
			return nil, err
		}
	}

	if args.Charm.Meta().Subordinate {
		return svc, nil
	}
	_, err = conn.AddUnits(svc, args.NumUnits)
	if err != nil {
		return nil, err
	}
	return svc, nil
}

func (conn *Conn) addCharm(curl *charm.URL, ch charm.Charm) (*state.Charm, error) {
	var f *os.File
	name := charm.Quote(curl.String())
	switch ch := ch.(type) {
	case *charm.Dir:
		var err error
		if f, err = ioutil.TempFile("", name); err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		defer f.Close()
		err = ch.BundleTo(f)
		if err != nil {
			return nil, fmt.Errorf("cannot bundle charm: %v", err)
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil, err
		}
	case *charm.Bundle:
		var err error
		if f, err = os.Open(ch.Path); err != nil {
			return nil, fmt.Errorf("cannot read charm bundle: %v", err)
		}
		defer f.Close()
	default:
		return nil, fmt.Errorf("unknown charm type %T", ch)
	}
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return nil, err
	}
	digest := hex.EncodeToString(h.Sum(nil))
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	storage := conn.Environ.Storage()
	log.Infof("writing charm to storage [%d bytes]", size)
	if err := storage.Put(name, f, size); err != nil {
		return nil, fmt.Errorf("cannot put charm: %v", err)
	}
	ustr, err := storage.URL(name)
	if err != nil {
		return nil, fmt.Errorf("cannot get storage URL for charm: %v", err)
	}
	u, err := url.Parse(ustr)
	if err != nil {
		return nil, fmt.Errorf("cannot parse storage URL: %v", err)
	}
	log.Infof("adding charm to state")
	sch, err := conn.State.AddCharm(ch, curl, u, digest)
	if err != nil {
		return nil, fmt.Errorf("cannot add charm: %v", err)
	}
	return sch, nil
}

// AddUnits starts n units of the given service and allocates machines
// to them as necessary.
func (conn *Conn) AddUnits(svc *state.Service, n int) ([]*state.Unit, error) {
	units := make([]*state.Unit, n)
	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		policy := conn.Environ.AssignmentPolicy()
		unit, err := svc.AddUnit()
		if err != nil {
			return nil, fmt.Errorf("cannot add unit %d/%d to service %q: %v", i+1, n, svc.Name(), err)
		}
		// TODO lp:1101139 (units are not assigned transactionally)
		if err := conn.State.AssignUnit(unit, policy); err != nil {
			return nil, err
		}
		units[i] = unit
	}
	return units, nil
}

// DestroyMachines destroys the specified machines.
func (conn *Conn) DestroyMachines(ids ...string) (err error) {
	var errs []string
	for _, id := range ids {
		machine, err := conn.State.Machine(id)
		switch {
		case state.IsNotFound(err):
			err = fmt.Errorf("machine %s does not exist", id)
		case err != nil:
		case machine.Life() != state.Alive:
			continue
		default:
			err = machine.Destroy()
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return destroyErr("machines", ids, errs)
}

// DestroyUnits destroys the specified units.
func (conn *Conn) DestroyUnits(names ...string) (err error) {
	var errs []string
	for _, name := range names {
		unit, err := conn.State.Unit(name)
		switch {
		case state.IsNotFound(err):
			err = fmt.Errorf("unit %q does not exist", name)
		case err != nil:
		case unit.Life() != state.Alive:
			continue
		case unit.IsPrincipal():
			err = unit.Destroy()
		default:
			err = fmt.Errorf("unit %q is a subordinate", name)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return destroyErr("units", names, errs)
}

func destroyErr(desc string, ids, errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	msg := "some %s were not destroyed"
	if len(errs) == len(ids) {
		msg = "no %s were destroyed"
	}
	msg = fmt.Sprintf(msg, desc)
	return fmt.Errorf("%s: %s", msg, strings.Join(errs, "; "))
}

// Resolved marks the unit as having had any previous state transition
// problems resolved, and informs the unit that it may attempt to
// reestablish normal workflow. The retryHooks parameter informs
// whether to attempt to reexecute previous failed hooks or to continue
// as if they had succeeded before.
func (conn *Conn) Resolved(unit *state.Unit, retryHooks bool) error {
	status, _, err := unit.Status()
	if err != nil {
		return err
	}
	if status != state.UnitError {
		return fmt.Errorf("unit %q is not in an error state", unit)
	}
	mode := state.ResolvedNoHooks
	if retryHooks {
		mode = state.ResolvedRetryHooks
	}
	return unit.SetResolved(mode)
}

// ServiceSet changes a service's configuration values.
// Values set to the empty string will be deleted.
func ServiceSet(st *state.State, p params.ServiceSet) error {
	return serviceSet(st, p.ServiceName, p.Options)
}

// ServiceSetYAML is like ServiceSet except that the
// configuration data is specified in YAML format.
func ServiceSetYAML(st *state.State, p params.ServiceSetYAML) error {
	// TODO(rog) should this function interpret null as delete?
	// If so, we need to sort out some goyaml issues first.
	// (see https://bugs.launchpad.net/goyaml/+bug/1133337)
	var options map[string]string
	if err := goyaml.Unmarshal([]byte(p.Config), &options); err != nil {
		return err
	}
	return serviceSet(st, p.ServiceName, options)
}

func serviceSet(st *state.State, svcName string, options map[string]string) error {
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
	srv, err := st.Service(svcName)
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
		cfg.Update(validated)
	}
	// 4. Delete any removed keys.
	if len(remove) > 0 {
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
