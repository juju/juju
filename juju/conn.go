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
	"time"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/ssh"
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
		info.Password = utils.UserPasswordHash(password, utils.CompatSalt)

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
	newcfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return err
	}
	return c.State.SetEnvironConfig(newcfg, cfg)
}

// PutCharm uploads the given charm to provider storage, and adds a
// state.Charm to the state.  The charm is not uploaded if a charm with
// the same URL already exists in the state.
// If bumpRevision is true, the charm must be a local directory,
// and the revision number will be incremented before pushing.
func (conn *Conn) PutCharm(curl *charm.URL, repo charm.Repository, bumpRevision bool) (*state.Charm, error) {
	if curl.Revision == -1 {
		rev, err := charm.Latest(repo, curl)
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

// InitJujuHome initializes the charm, environs/config and utils/ssh packages
// to use default paths based on the $JUJU_HOME or $HOME environment variables.
// This function should be called before calling NewConn or Conn.Deploy.
func InitJujuHome() error {
	jujuHome := osenv.JujuHomeDir()
	if jujuHome == "" {
		return stderrors.New(
			"cannot determine juju home, required environment variables are not set")
	}
	osenv.SetJujuHome(jujuHome)
	charm.CacheDir = osenv.JujuHomePath("charmcache")
	if err := ssh.LoadClientKeys(osenv.JujuHomePath("ssh")); err != nil {
		return fmt.Errorf("cannot load ssh client keys: %v", err)
	}
	return nil
}
