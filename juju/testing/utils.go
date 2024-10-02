// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"
	"os"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v4"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/network"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/charm"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/objectstore"
	objectstoretesting "github.com/juju/juju/internal/objectstore/testing"
	"github.com/juju/juju/state"
)

// PutCharm uploads the given charm to provider storage, and adds a
// state.Charm to the state.  The charm is not uploaded if a charm with
// the same URL already exists in the state.
func PutCharm(st *state.State, objectStore coreobjectstore.ObjectStore, curl *charm.URL, ch *charm.CharmDir) (*state.Charm, error) {
	if curl.Revision == -1 {
		curl.Revision = ch.Revision()
	}
	if sch, err := st.Charm(curl.String()); err == nil {
		return sch, nil
	}
	return AddCharm(st, objectStore, curl.String(), ch, false)
}

// AddCharm adds the charm to state and storage.
func AddCharm(st *state.State, objectStore coreobjectstore.ObjectStore, curl string, ch charm.Charm, force bool) (*state.Charm, error) {
	var f *os.File
	name := charm.Quote(curl)
	switch ch := ch.(type) {
	case *charm.CharmDir:
		var err error
		if f, err = os.CreateTemp("", name); err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		defer f.Close()
		err = ch.ArchiveTo(f)
		if err != nil {
			return nil, fmt.Errorf("cannot bundle charm: %v", err)
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil, err
		}
	case *charm.CharmArchive:
		var err error
		if f, err = os.Open(ch.Path); err != nil {
			return nil, fmt.Errorf("cannot read charm bundle: %v", err)
		}
		defer f.Close()
	default:
		return nil, fmt.Errorf("unknown charm type %T", ch)
	}
	digest, size, err := utils.ReadSHA256(f)
	if err != nil {
		return nil, err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}

	// ValidateLXDProfile is used here to replicate the same flow as the
	// not testing version.
	if err := lxdprofile.ValidateLXDProfile(lxdCharmProfiler{
		Charm: ch,
	}); err != nil && !force {
		return nil, err
	}

	storagePath := fmt.Sprintf("/charms/%s-%s", curl, digest)
	if err := objectStore.Put(context.Background(), storagePath, f, size); err != nil {
		return nil, fmt.Errorf("cannot put charm: %v", err)
	}
	info := state.CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: storagePath,
		SHA256:      digest,
	}
	sch, err := st.AddCharm(info)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot add charm")
	}
	return sch, nil
}

func NewObjectStore(c *gc.C, modelUUID string) coreobjectstore.ObjectStore {
	return NewObjectStoreWithMetadataService(c, modelUUID, objectstoretesting.MemoryMetadataService())
}

func NewObjectStoreWithMetadataService(c *gc.C, modelUUID string, metadataService objectstore.MetadataService) coreobjectstore.ObjectStore {
	store, err := objectstore.ObjectStoreFactory(
		context.Background(),
		objectstore.DefaultBackendType(),
		modelUUID,
		objectstore.WithRootDir(c.MkDir()),
		objectstore.WithLogger(loggertesting.WrapCheckLog(c)),

		// TODO (stickupkid): Swap this over to the real metadata service
		// when all facades are moved across.
		objectstore.WithMetadataService(metadataService),
		objectstore.WithClaimer(objectstoretesting.MemoryClaimer()),
	)
	c.Assert(err, jc.ErrorIsNil)
	return store
}

// lxdCharmProfiler massages a charm.Charm into a LXDProfiler inside of the
// core package.
type lxdCharmProfiler struct {
	Charm charm.Charm
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.Charm == nil {
		return nil
	}
	if profiler, ok := p.Charm.(charm.LXDProfiler); ok {
		profile := profiler.LXDProfile()
		if profile == nil {
			return nil
		}
		return profile
	}
	return nil
}

// AddControllerMachine adds a "controller" machine to the state so
// that State.Addresses and State.APIAddresses will work. It returns the
// added machine. The addresses that those methods will return bear no
// relation to the addresses actually used by the state and API servers.
// It returns the addresses that will be returned by the State.Addresses
// and State.APIAddresses methods, which will not bear any relation to
// the be the addresses used by the controllers.
func AddControllerMachine(
	c *gc.C,
	st *state.State,
	modelConfigService state.ModelConfigService,
	controllerConfig controller.Config,
) *state.Machine {
	machine, err := st.AddMachine(modelConfigService, state.UbuntuBase("12.10"), state.JobManageModel)
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProviderAddresses(controllerConfig, network.NewSpaceAddress("0.1.2.3"))
	c.Assert(err, jc.ErrorIsNil)

	hostPorts := []network.SpaceHostPorts{network.NewSpaceHostPorts(1234, "0.1.2.3")}
	err = st.SetAPIHostPorts(controllerConfig, hostPorts, hostPorts)
	c.Assert(err, jc.ErrorIsNil)

	return machine
}
