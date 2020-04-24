// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	"fmt"
	"regexp"
	"time"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type factorySuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&factorySuite{})

func (s *factorySuite) SetUpTest(c *gc.C) {
	s.NewPolicy = func(*state.State) state.Policy {
		return &statetesting.MockPolicy{
			GetStorageProviderRegistry: func() (storage.ProviderRegistry, error) {
				return provider.CommonStorageProviders(), nil
			},
		}
	}
	s.StateSuite.SetUpTest(c)
}

func (s *factorySuite) TestMakeUserNil(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	c.Assert(user.IsDisabled(), jc.IsFalse)

	saved, err := s.State.User(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Tag(), gc.Equals, user.Tag())
	c.Assert(saved.Name(), gc.Equals, user.Name())
	c.Assert(saved.DisplayName(), gc.Equals, user.DisplayName())
	c.Assert(saved.CreatedBy(), gc.Equals, user.CreatedBy())
	c.Assert(saved.DateCreated(), gc.Equals, user.DateCreated())
	c.Assert(saved.IsDisabled(), gc.Equals, user.IsDisabled())

	savedLastLogin, err := saved.LastLogin()
	c.Assert(err, jc.Satisfies, state.IsNeverLoggedInError)
	lastLogin, err := user.LastLogin()
	c.Assert(err, jc.Satisfies, state.IsNeverLoggedInError)
	c.Assert(savedLastLogin, gc.Equals, lastLogin)
}

func (s *factorySuite) TestMakeUserParams(c *gc.C) {
	username := "bob"
	displayName := "Bob the Builder"
	creator := s.Factory.MakeUser(c, nil)
	password := "sekrit"
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        username,
		DisplayName: displayName,
		Creator:     creator.Tag(),
		Password:    password,
	})
	c.Assert(user.IsDisabled(), jc.IsFalse)
	c.Assert(user.Name(), gc.Equals, username)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.CreatedBy(), gc.Equals, creator.UserTag().Name())
	c.Assert(user.PasswordValid(password), jc.IsTrue)

	saved, err := s.State.User(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Tag(), gc.Equals, user.Tag())
	c.Assert(saved.Name(), gc.Equals, user.Name())
	c.Assert(saved.DisplayName(), gc.Equals, user.DisplayName())
	c.Assert(saved.CreatedBy(), gc.Equals, user.CreatedBy())
	c.Assert(saved.DateCreated(), gc.Equals, user.DateCreated())
	c.Assert(saved.IsDisabled(), gc.Equals, user.IsDisabled())

	savedLastLogin, err := saved.LastLogin()
	c.Assert(err, jc.Satisfies, state.IsNeverLoggedInError)
	lastLogin, err := user.LastLogin()
	c.Assert(err, jc.Satisfies, state.IsNeverLoggedInError)
	c.Assert(savedLastLogin, gc.Equals, lastLogin)

	_, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *factorySuite) TestMakeUserInvalidCreator(c *gc.C) {
	invalidFunc := func() {
		s.Factory.MakeUser(c, &factory.UserParams{
			Name:        "bob",
			DisplayName: "Bob",
			Creator:     names.NewMachineTag("0"),
			Password:    "bob",
		})
	}

	c.Assert(invalidFunc, gc.PanicMatches, `interface conversion: .*`)
	saved, err := s.State.User(names.NewUserTag("bob"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(saved, gc.IsNil)
}

func (s *factorySuite) TestMakeUserNoModelUser(c *gc.C) {
	username := "bob"
	displayName := "Bob the Builder"
	creator := names.NewLocalUserTag("eric")
	password := "sekrit"
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        username,
		DisplayName: displayName,
		Creator:     creator,
		Password:    password,
		NoModelUser: true,
	})

	_, err := s.State.User(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *factorySuite) TestMakeModelUserNil(c *gc.C) {
	modelUser := s.Factory.MakeModelUser(c, nil)
	saved, err := s.State.UserAccess(modelUser.UserTag, modelUser.Object)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Object.Id(), gc.Equals, modelUser.Object.Id())
	c.Assert(saved.UserName, gc.Equals, modelUser.UserName)
	c.Assert(saved.DisplayName, gc.Equals, modelUser.DisplayName)
	c.Assert(saved.CreatedBy, gc.Equals, modelUser.CreatedBy)
}

func (s *factorySuite) TestMakeModelUserPartialParams(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar123", NoModelUser: true})
	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User: "foobar123"})

	saved, err := s.State.UserAccess(modelUser.UserTag, modelUser.Object)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Object.Id(), gc.Equals, modelUser.Object.Id())
	c.Assert(saved.UserName, gc.Equals, "foobar123")
	c.Assert(saved.DisplayName, gc.Equals, modelUser.DisplayName)
	c.Assert(saved.CreatedBy, gc.Equals, modelUser.CreatedBy)
}

func (s *factorySuite) TestMakeModelUserParams(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "foobar",
		Creator:     names.NewUserTag("createdby"),
		NoModelUser: true,
	})

	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User:        "foobar",
		CreatedBy:   names.NewUserTag("createdby"),
		DisplayName: "Foo Bar",
	})

	saved, err := s.State.UserAccess(modelUser.UserTag, s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Object.Id(), gc.Equals, modelUser.Object.Id())
	c.Assert(saved.UserName, gc.Equals, "foobar")
	c.Assert(saved.CreatedBy.Id(), gc.Equals, "createdby")
	c.Assert(saved.DisplayName, gc.Equals, "Foo Bar")
}

func (s *factorySuite) TestMakeModelUserInvalidCreatedBy(c *gc.C) {
	invalidFunc := func() {
		s.Factory.MakeModelUser(c, &factory.ModelUserParams{
			User:      "bob",
			CreatedBy: names.NewMachineTag("0"),
		})
	}

	c.Assert(invalidFunc, gc.PanicMatches, `interface conversion: .*`)
	saved, err := s.State.UserAccess(names.NewLocalUserTag("bob"), s.Model.ModelTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(saved, gc.DeepEquals, permission.UserAccess{})
}

func (s *factorySuite) TestMakeModelUserNonLocalUser(c *gc.C) {
	creator := s.Factory.MakeUser(c, &factory.UserParams{Name: "created-by"})
	modelUser := s.Factory.MakeModelUser(c, &factory.ModelUserParams{
		User:        "foobar@ubuntuone",
		DisplayName: "Foo Bar",
		CreatedBy:   creator.UserTag(),
	})

	saved, err := s.State.UserAccess(modelUser.UserTag, s.Model.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.Object.Id(), gc.Equals, modelUser.Object.Id())
	c.Assert(saved.UserName, gc.Equals, "foobar@ubuntuone")
	c.Assert(saved.DisplayName, gc.Equals, "Foo Bar")
	c.Assert(saved.CreatedBy.Id(), gc.Equals, creator.UserTag().Id())
}

func (s *factorySuite) TestMakeMachineNil(c *gc.C) {
	machine, password := s.Factory.MakeMachineReturningPassword(c, nil)
	c.Assert(machine, gc.NotNil)

	saved, err := s.State.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Series(), gc.Equals, machine.Series())
	c.Assert(saved.Id(), gc.Equals, machine.Id())
	c.Assert(saved.Series(), gc.Equals, machine.Series())
	c.Assert(saved.Tag(), gc.Equals, machine.Tag())
	c.Assert(saved.Life(), gc.Equals, machine.Life())
	c.Assert(saved.Jobs(), gc.DeepEquals, machine.Jobs())
	c.Assert(saved.PasswordValid(password), jc.IsTrue)
	savedInstanceId, err := saved.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedInstanceId, gc.Equals, machineInstanceId)
	c.Assert(saved.Clean(), gc.Equals, machine.Clean())
}

func (s *factorySuite) TestMakeMachine(c *gc.C) {
	series := "quantal"
	jobs := []state.MachineJob{state.JobManageModel}
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	nonce := "some-nonce"
	id := instance.Id("some-id")
	volumes := []state.HostVolumeParams{{Volume: state.VolumeParams{Size: 1024}}}
	filesystems := []state.HostFilesystemParams{{
		Filesystem: state.FilesystemParams{Pool: "loop", Size: 2048},
	}}

	machine, pwd := s.Factory.MakeMachineReturningPassword(c, &factory.MachineParams{
		Series:      series,
		Jobs:        jobs,
		Password:    password,
		Nonce:       nonce,
		InstanceId:  id,
		Volumes:     volumes,
		Filesystems: filesystems,
	})
	c.Assert(machine, gc.NotNil)
	c.Assert(pwd, gc.Equals, password)

	c.Assert(machine.Series(), gc.Equals, series)
	c.Assert(machine.Jobs(), gc.DeepEquals, jobs)
	machineInstanceId, err := machine.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(machineInstanceId, gc.Equals, id)
	c.Assert(machine.CheckProvisioned(nonce), jc.IsTrue)
	c.Assert(machine.PasswordValid(password), jc.IsTrue)

	sb, err := state.NewStorageBackend(s.State)
	c.Assert(err, jc.ErrorIsNil)
	assertVolume := func(name string, size uint64) {
		volume, err := sb.Volume(names.NewVolumeTag(name))
		c.Assert(err, jc.ErrorIsNil)
		volParams, ok := volume.Params()
		c.Assert(ok, jc.IsTrue)
		c.Assert(volParams, jc.DeepEquals, state.VolumeParams{Pool: "loop", Size: size})
		volAttachments, err := sb.VolumeAttachments(volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volAttachments, gc.HasLen, 1)
		c.Assert(volAttachments[0].Host(), gc.Equals, machine.Tag())
	}
	assertVolume(machine.Id()+"/0", 2048) // backing the filesystem
	assertVolume(machine.Id()+"/1", 1024)

	filesystem, err := sb.Filesystem(names.NewFilesystemTag(machine.Id() + "/0"))
	c.Assert(err, jc.ErrorIsNil)
	fsParams, ok := filesystem.Params()
	c.Assert(ok, jc.IsTrue)
	c.Assert(fsParams, jc.DeepEquals, state.FilesystemParams{Pool: "loop", Size: 2048})
	fsAttachments, err := sb.MachineFilesystemAttachments(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsAttachments, gc.HasLen, 1)
	c.Assert(fsAttachments[0].Host(), gc.Equals, machine.Tag())

	saved, err := s.State.Machine(machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Id(), gc.Equals, machine.Id())
	c.Assert(saved.Series(), gc.Equals, machine.Series())
	c.Assert(saved.Tag(), gc.Equals, machine.Tag())
	c.Assert(saved.Life(), gc.Equals, machine.Life())
	c.Assert(saved.Jobs(), gc.DeepEquals, machine.Jobs())
	savedInstanceId, err := saved.InstanceId()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(savedInstanceId, gc.Equals, machineInstanceId)
	c.Assert(saved.Clean(), gc.Equals, machine.Clean())
}

func (s *factorySuite) TestMakeCharmNil(c *gc.C) {
	charm := s.Factory.MakeCharm(c, nil)
	c.Assert(charm, gc.NotNil)

	saved, err := s.State.Charm(charm.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.URL(), gc.DeepEquals, charm.URL())
	c.Assert(saved.Meta(), gc.DeepEquals, charm.Meta())
	c.Assert(saved.StoragePath(), gc.Equals, charm.StoragePath())
	c.Assert(saved.BundleSha256(), gc.Equals, charm.BundleSha256())
}

func (s *factorySuite) TestMakeCharm(c *gc.C) {
	series := "quantal"
	name := "wordpress"
	revision := 13
	url := fmt.Sprintf("cs:%s/%s-%d", series, name, revision)
	ch := s.Factory.MakeCharm(c, &factory.CharmParams{
		Name: name,
		URL:  url,
	})
	c.Assert(ch, gc.NotNil)

	c.Assert(ch.URL(), gc.DeepEquals, charm.MustParseURL(url))

	saved, err := s.State.Charm(ch.URL())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.URL(), gc.DeepEquals, ch.URL())
	c.Assert(saved.Meta(), gc.DeepEquals, ch.Meta())
	c.Assert(saved.Meta().Name, gc.Equals, name)
	c.Assert(saved.StoragePath(), gc.Equals, ch.StoragePath())
	c.Assert(saved.BundleSha256(), gc.Equals, ch.BundleSha256())
}

func (s *factorySuite) TestMakeApplicationNil(c *gc.C) {
	application := s.Factory.MakeApplication(c, nil)
	c.Assert(application, gc.NotNil)

	saved, err := s.State.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Name(), gc.Equals, application.Name())
	c.Assert(saved.Tag(), gc.Equals, application.Tag())
	c.Assert(saved.Life(), gc.Equals, application.Life())
}

func (s *factorySuite) TestMakeApplication(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	application := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Charm: charm,
	})
	c.Assert(application, gc.NotNil)

	c.Assert(application.Name(), gc.Equals, "wordpress")
	curl, _ := application.CharmURL()
	c.Assert(curl, gc.DeepEquals, charm.URL())

	saved, err := s.State.Application(application.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Name(), gc.Equals, application.Name())
	c.Assert(saved.Tag(), gc.Equals, application.Tag())
	c.Assert(saved.Life(), gc.Equals, application.Life())
}

func (s *factorySuite) TestMakeUnitNil(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	c.Assert(unit, gc.NotNil)

	saved, err := s.State.Unit(unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Name(), gc.Equals, unit.Name())
	c.Assert(saved.ApplicationName(), gc.Equals, unit.ApplicationName())
	c.Assert(saved.Series(), gc.Equals, unit.Series())
	c.Assert(saved.Life(), gc.Equals, unit.Life())
}

func (s *factorySuite) TestMakeUnit(c *gc.C) {
	application := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Application: application,
		SetCharmURL: true,
	})
	c.Assert(unit, gc.NotNil)

	c.Assert(unit.ApplicationName(), gc.Equals, application.Name())

	saved, err := s.State.Unit(unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Name(), gc.Equals, unit.Name())
	c.Assert(saved.ApplicationName(), gc.Equals, unit.ApplicationName())
	c.Assert(saved.Series(), gc.Equals, unit.Series())
	c.Assert(saved.Life(), gc.Equals, unit.Life())

	applicationCharmURL, _ := application.CharmURL()
	unitCharmURL, _ := saved.CharmURL()
	c.Assert(unitCharmURL, gc.DeepEquals, applicationCharmURL)
}

func (s *factorySuite) TestMakeRelationNil(c *gc.C) {
	relation := s.Factory.MakeRelation(c, nil)
	c.Assert(relation, gc.NotNil)

	saved, err := s.State.Relation(relation.Id())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Id(), gc.Equals, relation.Id())
	c.Assert(saved.Tag(), gc.Equals, relation.Tag())
	c.Assert(saved.Life(), gc.Equals, relation.Life())
	c.Assert(saved.Endpoints(), gc.DeepEquals, relation.Endpoints())
}

func (s *factorySuite) TestMakeRelation(c *gc.C) {
	s1 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "application1",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := s1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	s2 := s.Factory.MakeApplication(c, &factory.ApplicationParams{
		Name: "application2",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "mysql",
		}),
	})
	e2, err := s2.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)

	relation := s.Factory.MakeRelation(c, &factory.RelationParams{
		Endpoints: []state.Endpoint{e1, e2},
	})
	c.Assert(relation, gc.NotNil)

	saved, err := s.State.Relation(relation.Id())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Id(), gc.Equals, relation.Id())
	c.Assert(saved.Tag(), gc.Equals, relation.Tag())
	c.Assert(saved.Life(), gc.Equals, relation.Life())
	c.Assert(saved.Endpoints(), gc.DeepEquals, relation.Endpoints())
}

func (s *factorySuite) TestMakeMetricNil(c *gc.C) {
	metric := s.Factory.MakeMetric(c, nil)
	c.Assert(metric, gc.NotNil)

	saved, err := s.State.MetricBatch(metric.UUID())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.UUID(), gc.Equals, metric.UUID())
	c.Assert(saved.Unit(), gc.Equals, metric.Unit())
	c.Assert(saved.Sent(), gc.Equals, metric.Sent())
	c.Assert(saved.CharmURL(), gc.Equals, metric.CharmURL())
	c.Assert(saved.Sent(), gc.Equals, metric.Sent())
	c.Assert(saved.Metrics(), gc.HasLen, 1)
	c.Assert(saved.Metrics()[0].Key, gc.Equals, metric.Metrics()[0].Key)
	c.Assert(saved.Metrics()[0].Value, gc.Equals, metric.Metrics()[0].Value)
	c.Assert(saved.Metrics()[0].Time.Equal(metric.Metrics()[0].Time), jc.IsTrue)
}

func (s *factorySuite) TestMakeMetric(c *gc.C) {
	now := time.Now().Round(time.Second).UTC()
	meteredCharm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "metered", URL: "cs:quantal/metered"})
	meteredApplication := s.Factory.MakeApplication(c, &factory.ApplicationParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: meteredApplication, SetCharmURL: true})
	metric := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit:    unit,
		Time:    &now,
		Sent:    true,
		Metrics: []state.Metric{{Key: "pings", Value: "1", Time: now}},
	})
	c.Assert(metric, gc.NotNil)

	saved, err := s.State.MetricBatch(metric.UUID())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.UUID(), gc.Equals, metric.UUID())
	c.Assert(saved.Unit(), gc.Equals, metric.Unit())
	c.Assert(saved.CharmURL(), gc.Equals, metric.CharmURL())
	c.Assert(metric.Sent(), jc.IsTrue)
	c.Assert(saved.Sent(), jc.IsTrue)
	c.Assert(saved.Metrics(), gc.HasLen, 1)
	c.Assert(saved.Metrics()[0].Key, gc.Equals, "pings")
	c.Assert(saved.Metrics()[0].Value, gc.Equals, "1")
	c.Assert(saved.Metrics()[0].Time.Equal(now), jc.IsTrue)
}

func (s *factorySuite) TestMakeModelNil(c *gc.C) {
	st := s.Factory.MakeModel(c, nil)
	defer st.Close()

	env, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	re := regexp.MustCompile(`^testmodel-\d+$`)
	c.Assert(re.MatchString(env.Name()), jc.IsTrue)
	c.Assert(env.UUID() == s.State.ModelUUID(), jc.IsFalse)
	origEnv, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Owner(), gc.Equals, origEnv.Owner())

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["default-series"], gc.Equals, "bionic")
}

func (s *factorySuite) TestMakeModel(c *gc.C) {
	owner := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "owner",
	})
	params := &factory.ModelParams{
		Name:        "foo",
		Owner:       owner.UserTag(),
		ConfigAttrs: testing.Attrs{"default-series": "precise"},
	}

	st := s.Factory.MakeModel(c, params)
	defer st.Close()

	env, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Name(), gc.Equals, "foo")
	c.Assert(env.UUID() == s.State.ModelUUID(), jc.IsFalse)
	c.Assert(env.Owner(), gc.Equals, owner.UserTag())

	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	cfg, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["default-series"], gc.Equals, "precise")
}
