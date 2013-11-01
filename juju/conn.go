// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"crypto/sha256"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
)

// Conn holds a connection to a juju environment and its
// associated state.
type Conn struct {
	Environ environs.Environ
	State   *state.State
}

var redialStrategy = utils.AttemptStrategy{
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
	err = environs.CheckEnvironment(environ)
	if err != nil {
		return nil, err
	}

	info.Password = password
	opts := state.DefaultDialOpts()
	st, err := state.Open(info, opts)
	if errors.IsUnauthorizedError(err) {
		log.Noticef("juju: authorization error while connecting to state server; retrying")
		// We can't connect with the administrator password,;
		// perhaps this was the first connection and the
		// password has not been changed yet.
		info.Password = utils.PasswordHash(password)

		// We try for a while because we might succeed in
		// connecting to mongo before the state has been
		// initialized and the initial password set.
		for a := redialStrategy.Start(); a.Next(); {
			st, err = state.Open(info, opts)
			if !errors.IsUnauthorizedError(err) {
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
	store, err := configstore.Default()
	if err != nil {
		return nil, err
	}
	environ, err := environs.NewFromName(environName, store)
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
	for k, v := range secrets {
		if _, exists := attrs[k]; exists {
			// Environment already has secrets. Won't send again.
			return nil
		} else {
			attrs[k] = v
		}
	}
	cfg, err = config.New(config.NoDefaults, attrs)
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
	ServiceName    string
	Charm          *state.Charm
	ConfigSettings charm.Settings
	Constraints    constraints.Value
	NumUnits       int
	// ToMachineSpec is either:
	// - an existing machine/container id eg "1" or "1/lxc/2"
	// - a new container on an existing machine eg "lxc:1"
	// Use string to avoid ambiguity around machine 0.
	ToMachineSpec string
}

// DeployService takes a charm and various parameters and deploys it.
func (conn *Conn) DeployService(args DeployServiceParams) (*state.Service, error) {
	if args.NumUnits > 1 && args.ToMachineSpec != "" {
		return nil, stderrors.New("cannot use --num-units with --to")
	}
	settings, err := args.Charm.Config().ValidateSettings(args.ConfigSettings)
	if err != nil {
		return nil, err
	}
	if args.Charm.Meta().Subordinate {
		if args.NumUnits != 0 || args.ToMachineSpec != "" {
			return nil, fmt.Errorf("subordinate service must be deployed without units")
		}
		if !constraints.IsEmpty(&args.Constraints) {
			return nil, fmt.Errorf("subordinate service must be deployed without constraints")
		}
	}
	// TODO(fwereade): transactional State.AddService including settings, constraints
	// (minimumUnitCount, initialMachineIds?).
	service, err := conn.State.AddService(args.ServiceName, args.Charm)
	if err != nil {
		return nil, err
	}
	if len(settings) > 0 {
		if err := service.UpdateConfigSettings(settings); err != nil {
			return nil, err
		}
	}
	if args.Charm.Meta().Subordinate {
		return service, nil
	}
	if !constraints.IsEmpty(&args.Constraints) {
		if err := service.SetConstraints(args.Constraints); err != nil {
			return nil, err
		}
	}
	if args.NumUnits > 0 {
		if _, err := conn.AddUnits(service, args.NumUnits, args.ToMachineSpec); err != nil {
			return nil, err
		}
	}
	return service, nil
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
	stor := conn.Environ.Storage()
	log.Infof("writing charm to storage [%d bytes]", size)
	if err := stor.Put(name, f, size); err != nil {
		return nil, fmt.Errorf("cannot put charm: %v", err)
	}
	ustr, err := stor.URL(name)
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
func (conn *Conn) AddUnits(svc *state.Service, n int, machineIdSpec string) ([]*state.Unit, error) {
	units := make([]*state.Unit, n)
	// Hard code for now till we implement a different approach.
	policy := state.AssignCleanEmpty
	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		unit, err := svc.AddUnit()
		if err != nil {
			return nil, fmt.Errorf("cannot add unit %d/%d to service %q: %v", i+1, n, svc.Name(), err)
		}
		if machineIdSpec != "" {
			if n != 1 {
				return nil, fmt.Errorf("cannot add multiple units of service %q to a single machine", svc.Name())
			}
			// machineIdSpec may be an existing machine or container, eg 3/lxc/2
			// or a new container on a machine, eg lxc:3
			mid := machineIdSpec
			var containerType instance.ContainerType
			specParts := strings.Split(machineIdSpec, ":")
			if len(specParts) > 1 {
				firstPart := specParts[0]
				var err error
				if containerType, err = instance.ParseSupportedContainerType(firstPart); err == nil {
					mid = strings.Join(specParts[1:], "/")
				} else {
					mid = machineIdSpec
				}
			}
			if !names.IsMachine(mid) {
				return nil, fmt.Errorf("invalid force machine id %q", mid)
			}

			var err error
			var m *state.Machine
			// If a container is to be used, create it.
			if containerType != "" {
				params := state.AddMachineParams{
					Series:        unit.Series(),
					ParentId:      mid,
					ContainerType: containerType,
					Jobs:          []state.MachineJob{state.JobHostUnits},
				}
				m, err = conn.State.AddMachineWithConstraints(&params)

			} else {
				m, err = conn.State.Machine(mid)
			}
			if err != nil {
				return nil, fmt.Errorf("cannot assign unit %q to machine: %v", unit.Name(), err)
			}
			err = unit.AssignToMachine(m)

			if err != nil {
				return nil, err
			}
		} else if err := conn.State.AssignUnit(unit, policy); err != nil {
			return nil, err
		}
		units[i] = unit
	}
	return units, nil
}

// InitJujuHome initializes the charm and environs/config packages to use
// default paths based on the $JUJU_HOME or $HOME environment variables.
// This function should be called before calling NewConn or Conn.Deploy.
func InitJujuHome() error {
	jujuHome := osenv.JujuHomeDir()
	if jujuHome == "" {
		return stderrors.New(
			"cannot determine juju home, required environment variables are not set")
	}
	config.SetJujuHome(jujuHome)
	charm.CacheDir = filepath.Join(jujuHome, "charmcache")
	return nil
}
