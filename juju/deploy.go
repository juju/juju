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

// NewService creates a new service with the given name to run the given
// charm.  If svcName is empty, the charm name will be used.
func (conn *Conn) NewService(sch *state.Charm, svcName string) (*state.Service, error) {
	st, err := conn.State()
	if err != nil {
		return nil, err
	}
	if svcName == "" {
		svcName = sch.URL().Name // TODO sch.Meta().Name ?
	}
	svc, err := st.AddService(svcName, sch)
	if err != nil {
		return nil, err
	}
	meta := sch.Meta()
	for name, rel := range meta.Peers {
		ep := state.RelationEndpoint{
			svcName,
			rel.Interface,
			name,
			state.RolePeer,
			state.RelationScope(rel.Scope),
		}
		if err := st.AddRelation(ep); err != nil {
			return nil, fmt.Errorf("cannot add peer relation %q to service %q: %v", name, svcName, err)
		}
	}
	return svc, nil
}

// StartUnit starts a machine running n units of the given service.
func (conn *Conn) StartUnits(svc *state.Service, n int) ([]*state.Unit, error) {
	st, err := conn.State()
	if err != nil {
		return nil, err
	}
	units := make([]*state.Unit, n)
	// TODO what do we do if we fail half-way through this process?
	for i := 0; i < n; i++ {
		policy := conn.Environ.AssignmentPolicy()
		unit, err := svc.AddUnit()
		if err != nil {
			return nil, fmt.Errorf("cannot add unit %d/%d to service %q: %v", i+1, n, svc.Name(), err)
		}
		if err := st.AssignUnit(unit, policy); err != nil {
			return nil, fmt.Errorf("cannot assign machine to unit %s of service %q: %v", unit.Name(), svc.Name(), err)
		}
		units[i] = unit
	}
	return units, nil
}

// PutCharm uploads the given charm to provider storage,
// and adds a state.Charm to the state. The charm is not uploaded
// if a charm with the same URL already exists in the state,
// unless upgrade is true. If upgrade is true and the charm URL
// refers to a local directory, the revision number will be incremented
// before pushing. Local charms will be interpreted relative to the repoPath
// directory.
func (conn *Conn) PutCharm(curl *charm.URL, repoPath string, upgrade bool) (*state.Charm, error) {
	repo, err := charm.InferRepository(curl, repoPath)
	if err != nil {
		return nil, fmt.Errorf("cannot infer charm repository: %v", err)
	}
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
	if upgrade {
		chd, ok := ch.(*charm.Dir)
		if !ok {
			return nil, fmt.Errorf("cannot upgrade charm %q: not a directory", curl)
		}
		if err = chd.SetDiskRevision(chd.Revision() + 1); err != nil {
			return nil, fmt.Errorf("cannot upgrade charm %q: %v", curl, err)
		}
		curl = curl.WithRevision(chd.Revision())
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
	sch, err := st.AddCharm(ch, curl, u, digest)
	if err != nil {
		return nil, fmt.Errorf("cannot add charm: %v", err)
	}
	return sch, nil
}
