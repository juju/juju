package juju

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/state"
	"net/url"
	"os"
)

// AddService creates a new service with the given name to run the given
// charm.  If svcName is empty, the charm name will be used.
func (conn *Conn) AddService(name string, ch *state.Charm) (*state.Service, error) {
	if name == "" {
		name = ch.URL().Name // TODO ch.Meta().Name ?
	}
	svc, err := conn.State.AddService(name, ch)
	if err != nil {
		return nil, err
	}
	meta := ch.Meta()
	for rname, rel := range meta.Peers {
		ep := state.Endpoint{
			name,
			rel.Interface,
			rname,
			state.RolePeer,
			rel.Scope,
		}
		if _, err := conn.State.AddRelation(ep); err != nil {
			return nil, fmt.Errorf("cannot add peer relation %q to service %q: %v", rname, name, err)
		}
	}
	return svc, nil
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
			return nil, fmt.Errorf("cannot increment version of charm %q: not a directory", curl)
		}
		if err = chd.SetDiskRevision(chd.Revision() + 1); err != nil {
			return nil, fmt.Errorf("cannot increment version of charm %q: %v", curl, err)
		}
		curl = curl.WithRevision(chd.Revision())
	}
	if sch, err := conn.State.Charm(curl); err == nil {
		return sch, nil
	}
	var buf bytes.Buffer
	switch ch := ch.(type) {
	case *charm.Dir:
		if err := ch.BundleTo(&buf); err != nil {
			return nil, fmt.Errorf("cannot bundle charm: %v", err)
		}
	case *charm.Bundle:
		f, err := os.Open(ch.Path)
		if err != nil {
			return nil, fmt.Errorf("cannot open charm bundle path: %v", err)
		}
		defer f.Close()
		if _, err := io.Copy(&buf, f); err != nil {
			return nil, fmt.Errorf("cannot read charm from bundle: %v", err)
		}
	default:
		return nil, fmt.Errorf("unknown charm type %T", ch)
	}
	h := sha256.New()
	h.Write(buf.Bytes())
	digest := hex.EncodeToString(h.Sum(nil))
	storage := conn.Environ.Storage()
	name := charm.Quote(curl.String())
	if err := storage.Put(name, &buf, int64(len(buf.Bytes()))); err != nil {
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
		if err := conn.State.AssignUnit(unit, policy); err != nil {
			return nil, err
		}
		units[i] = unit
	}
	return units, nil
}

// DestroyUnits removes the specified units from the state.
func (conn *Conn) DestroyUnits(units ...*state.Unit) error {
	for _, unit := range units {
		if err := unit.EnsureDying(); err != nil {
			return err
		}
	}
	return nil
}
