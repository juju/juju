package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"net/url"
	"os"
)

type DeployCommand struct {
	EnvName      string
	CharmName    string
	ServiceName  string
	ConfPath     string
	NumUnits     int // defaults to 1
	BumpRevision bool
	RepoPath     string // defaults to JUJU_REPOSITORY
}

const deployDoc = `
<charm name> can be a charm URL, or an unambiguously condensed form of it;
assuming a current default series of "precise", the following forms will be
accepted.

For cs:precise/mysql
  mysql
  precise/mysql

For cs:~user/precise/mysql
  cs:~user/mysql

For local:precise/mysql
  local:mysql

In all cases, a versioned charm URL will be expanded as expected (for example,
mysql-33 becomes cs:precise/mysql-33).

<service name>, if omitted, will be derived from <charm name>.
`

func (c *DeployCommand) Info() *cmd.Info {
	return &cmd.Info{
		"deploy", "<charm name> [<service name>]", "deploy a new service", deployDoc,
	}
}

func (c *DeployCommand) Init(f *gnuflag.FlagSet, args []string) error {
	addEnvironFlags(&c.EnvName, f)
	f.IntVar(&c.NumUnits, "n", 1, "number of service units to deploy for principal charms")
	f.IntVar(&c.NumUnits, "num-units", 1, "")
	f.BoolVar(&c.BumpRevision, "u", false, "increment local charm directory revision")
	f.BoolVar(&c.BumpRevision, "upgrade", false, "")
	f.StringVar(&c.ConfPath, "config", "", "path to yaml-formatted service config")
	f.StringVar(&c.RepoPath, "repository", os.Getenv("JUJU_REPOSITORY"), "local charm repository")
	// TODO --constraints
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	switch len(args) {
	case 2:
		c.ServiceName = args[1]
		fallthrough
	case 1:
		c.CharmName = args[0]
	case 0:
		return errors.New("no charm specified")
	default:
		return cmd.CheckEmpty(args[2:])
	}
	if c.NumUnits < 1 {
		return errors.New("must deploy at least one unit")
	}
	return nil
}

// getCharm returns the charm and charm URL specified on the command line.
// If --upgrade is specified, the charm must be a local directory, and will
// have its version bumped before being returned.
func (c *DeployCommand) getCharm(defaultSeries string) (charm.Charm, *charm.URL, error) {
	repo, curl, err := charm.InferRepository(c.CharmName, defaultSeries, c.RepoPath)
	if err != nil {
		return nil, nil, err
	}
	if curl.Revision == -1 {
		rev, err := repo.Latest(curl)
		if err != nil {
			return nil, nil, err
		}
		curl = curl.WithRevision(rev)
	}
	ch, err := repo.Get(curl)
	if err != nil {
		return nil, nil, err
	}
	if c.BumpRevision {
		chd, ok := ch.(*charm.Dir)
		if !ok {
			return nil, nil, fmt.Errorf("can't upgrade: charm %q is not a directory", curl)
		}
		if err = chd.SetDiskRevision(chd.Revision() + 1); err != nil {
			return nil, nil, err
		}
		curl = curl.WithRevision(chd.Revision())
	}
	return ch, curl, nil
}

// putCharm uploads the charm specified on the command line to provider storage,
// and adds a state.Charm to the state.
func (c *DeployCommand) putCharm(conn *juju.Conn) (*state.Charm, error) {
	// TODO get default series from environ
	ch, curl, err := c.getCharm("precise")
	if err != nil {
		return nil, err
	}
	st, err := conn.State()
	if err != nil {
		return nil, err
	}
	if sch, err := st.Charm(curl); err == nil {
		return sch, nil
	}
	var buf bytes.Buffer
	switch ch := ch.(type) {
	case *charm.Dir:
		if err := ch.BundleTo(&buf); err != nil {
			return nil, err
		}
	case *charm.Bundle:
		f, err := os.Open(ch.Path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := io.Copy(&buf, f); err != nil {
			return nil, err
		}
	default:
		panic("unknown charm type")
	}
	h := sha256.New()
	h.Write(buf.Bytes())
	digest := hex.EncodeToString(h.Sum(nil))
	storage := conn.Environ.Storage()
	name := charm.Quote(curl.String())
	if err := storage.Put(name, &buf, int64(len(buf.Bytes()))); err != nil {
		return nil, err
	}
	ustr, err := storage.URL(name)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(ustr)
	if err != nil {
		return nil, err
	}
	return st.AddCharm(ch, curl, u, digest)
}

func (c *DeployCommand) Run(ctx *cmd.Context) error {
	conn, err := juju.NewConn(c.EnvName)
	if err != nil {
		return err
	}
	defer conn.Close()
	sch, err := c.putCharm(conn)
	if err != nil {
		return err
	}
	if c.ServiceName == "" {
		c.ServiceName = sch.URL().Name
	}
	st, err := conn.State()
	if err != nil {
		return err
	}
	srv, err := st.AddService(c.ServiceName, sch)
	if err != nil {
		return err
	}
	if c.ConfPath != "" {
		// TODO many dependencies :(
		panic("state.Service.SetConfig not implemented (format 2...)")
	}
	meta := sch.Meta()
	for name, rel := range meta.Peers {
		ep := state.RelationEndpoint{
			c.ServiceName,
			rel.Interface,
			name,
			state.RolePeer,
			state.RelationScope(rel.Scope),
		}
		if err := st.AddRelation(ep); err != nil {
			return err
		}
	}
	if !meta.Subordinate {
		policy := conn.Environ.AssignmentPolicy()
		for i := 0; i < c.NumUnits; i++ {
			unit, err := srv.AddUnit()
			if err != nil {
				return err
			}
			if err := st.AssignUnit(unit, policy); err != nil {
				return err
			}
		}
	}
	return nil
}
