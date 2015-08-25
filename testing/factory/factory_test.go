// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	"fmt"
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/storage/provider"
	"github.com/juju/juju/storage/provider/registry"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type factorySuite struct {
	statetesting.StateSuite
	Factory *factory.Factory
}

var _ = gc.Suite(&factorySuite{})

func (s *factorySuite) SetUpTest(c *gc.C) {
	s.Policy = new(statetesting.MockPolicy)
	s.StateSuite.SetUpTest(c)
	s.Factory = factory.NewFactory(s.State)
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

	_, err = s.State.EnvironmentUser(user.UserTag())
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

func (s *factorySuite) TestMakeUserNoEnvUser(c *gc.C) {
	username := "bob"
	displayName := "Bob the Builder"
	creator := names.NewLocalUserTag("eric")
	password := "sekrit"
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        username,
		DisplayName: displayName,
		Creator:     creator,
		Password:    password,
		NoEnvUser:   true,
	})

	_, err := s.State.User(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	_, err = s.State.EnvironmentUser(user.UserTag())
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *factorySuite) TestMakeEnvUserNil(c *gc.C) {
	envUser := s.Factory.MakeEnvUser(c, nil)
	saved, err := s.State.EnvironmentUser(envUser.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.EnvironmentTag().Id(), gc.Equals, envUser.EnvironmentTag().Id())
	c.Assert(saved.UserName(), gc.Equals, envUser.UserName())
	c.Assert(saved.DisplayName(), gc.Equals, envUser.DisplayName())
	c.Assert(saved.CreatedBy(), gc.Equals, envUser.CreatedBy())
}

func (s *factorySuite) TestMakeEnvUserPartialParams(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar123", NoEnvUser: true})
	envUser := s.Factory.MakeEnvUser(c, &factory.EnvUserParams{
		User: "foobar123"})

	saved, err := s.State.EnvironmentUser(envUser.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.EnvironmentTag().Id(), gc.Equals, envUser.EnvironmentTag().Id())
	c.Assert(saved.UserName(), gc.Equals, "foobar123@local")
	c.Assert(saved.DisplayName(), gc.Equals, envUser.DisplayName())
	c.Assert(saved.CreatedBy(), gc.Equals, envUser.CreatedBy())
}

func (s *factorySuite) TestMakeEnvUserParams(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "createdby"})
	s.Factory.MakeUser(c, &factory.UserParams{
		Name:      "foobar",
		Creator:   names.NewUserTag("createdby"),
		NoEnvUser: true,
	})

	envUser := s.Factory.MakeEnvUser(c, &factory.EnvUserParams{
		User:        "foobar",
		CreatedBy:   names.NewUserTag("createdby"),
		DisplayName: "Foo Bar",
	})

	saved, err := s.State.EnvironmentUser(envUser.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.EnvironmentTag().Id(), gc.Equals, envUser.EnvironmentTag().Id())
	c.Assert(saved.UserName(), gc.Equals, "foobar@local")
	c.Assert(saved.CreatedBy(), gc.Equals, "createdby@local")
	c.Assert(saved.DisplayName(), gc.Equals, "Foo Bar")
}

func (s *factorySuite) TestMakeEnvUserInvalidCreatedBy(c *gc.C) {
	invalidFunc := func() {
		s.Factory.MakeEnvUser(c, &factory.EnvUserParams{
			User:      "bob",
			CreatedBy: names.NewMachineTag("0"),
		})
	}

	c.Assert(invalidFunc, gc.PanicMatches, `interface conversion: .*`)
	saved, err := s.State.EnvironmentUser(names.NewLocalUserTag("bob"))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(saved, gc.IsNil)
}

func (s *factorySuite) TestMakeEnvUserNonLocalUser(c *gc.C) {
	creator := s.Factory.MakeUser(c, &factory.UserParams{Name: "created-by"})
	envUser := s.Factory.MakeEnvUser(c, &factory.EnvUserParams{
		User:        "foobar@ubuntuone",
		DisplayName: "Foo Bar",
		CreatedBy:   creator.UserTag(),
	})

	saved, err := s.State.EnvironmentUser(envUser.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved.EnvironmentTag().Id(), gc.Equals, envUser.EnvironmentTag().Id())
	c.Assert(saved.UserName(), gc.Equals, "foobar@ubuntuone")
	c.Assert(saved.DisplayName(), gc.Equals, "Foo Bar")
	c.Assert(saved.CreatedBy(), gc.Equals, creator.UserTag().Username())
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
	registry.RegisterEnvironStorageProviders("someprovider", provider.LoopProviderType)
	series := "quantal"
	jobs := []state.MachineJob{state.JobManageEnviron}
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	nonce := "some-nonce"
	id := instance.Id("some-id")
	volumes := []state.MachineVolumeParams{{Volume: state.VolumeParams{Size: 1024}}}
	filesystems := []state.MachineFilesystemParams{{
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

	assertVolume := func(name string, size uint64) {
		volume, err := s.State.Volume(names.NewVolumeTag(name))
		c.Assert(err, jc.ErrorIsNil)
		volParams, ok := volume.Params()
		c.Assert(ok, jc.IsTrue)
		c.Assert(volParams, jc.DeepEquals, state.VolumeParams{Pool: "loop", Size: size})
		volAttachments, err := s.State.VolumeAttachments(volume.VolumeTag())
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(volAttachments, gc.HasLen, 1)
		c.Assert(volAttachments[0].Machine(), gc.Equals, machine.Tag())
	}
	assertVolume(machine.Id()+"/0", 2048) // backing the filesystem
	assertVolume(machine.Id()+"/1", 1024)

	filesystem, err := s.State.Filesystem(names.NewFilesystemTag(machine.Id() + "/0"))
	c.Assert(err, jc.ErrorIsNil)
	fsParams, ok := filesystem.Params()
	c.Assert(ok, jc.IsTrue)
	c.Assert(fsParams, jc.DeepEquals, state.FilesystemParams{Pool: "loop", Size: 2048})
	fsAttachments, err := s.State.MachineFilesystemAttachments(machine.Tag().(names.MachineTag))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fsAttachments, gc.HasLen, 1)
	c.Assert(fsAttachments[0].Machine(), gc.Equals, machine.Tag())

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

func (s *factorySuite) TestMakeServiceNil(c *gc.C) {
	service := s.Factory.MakeService(c, nil)
	c.Assert(service, gc.NotNil)

	saved, err := s.State.Service(service.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Name(), gc.Equals, service.Name())
	c.Assert(saved.Tag(), gc.Equals, service.Tag())
	c.Assert(saved.Life(), gc.Equals, service.Life())
}

func (s *factorySuite) TestMakeService(c *gc.C) {
	charm := s.Factory.MakeCharm(c, &factory.CharmParams{Name: "wordpress"})
	creator := s.Factory.MakeUser(c, &factory.UserParams{Name: "bill"}).Tag()
	service := s.Factory.MakeService(c, &factory.ServiceParams{
		Charm:   charm,
		Creator: creator,
	})
	c.Assert(service, gc.NotNil)

	c.Assert(service.Name(), gc.Equals, "wordpress")
	c.Assert(service.GetOwnerTag(), gc.Equals, creator.String())
	curl, _ := service.CharmURL()
	c.Assert(curl, gc.DeepEquals, charm.URL())

	saved, err := s.State.Service(service.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Name(), gc.Equals, service.Name())
	c.Assert(saved.Tag(), gc.Equals, service.Tag())
	c.Assert(saved.Life(), gc.Equals, service.Life())
}

func (s *factorySuite) TestMakeServiceInvalidCreator(c *gc.C) {
	serviceName := "mysql"
	invalidFunc := func() {
		s.Factory.MakeService(c, &factory.ServiceParams{
			Name:    serviceName,
			Creator: names.NewMachineTag("0"),
		})
	}
	c.Assert(invalidFunc, gc.PanicMatches, `interface conversion: .*`)
	saved, err := s.State.Service(serviceName)
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(saved, gc.IsNil)
}

func (s *factorySuite) TestMakeUnitNil(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	c.Assert(unit, gc.NotNil)

	saved, err := s.State.Unit(unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Name(), gc.Equals, unit.Name())
	c.Assert(saved.ServiceName(), gc.Equals, unit.ServiceName())
	c.Assert(saved.Series(), gc.Equals, unit.Series())
	c.Assert(saved.Life(), gc.Equals, unit.Life())
}

func (s *factorySuite) TestMakeUnit(c *gc.C) {
	service := s.Factory.MakeService(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{
		Service:     service,
		SetCharmURL: true,
	})
	c.Assert(unit, gc.NotNil)

	c.Assert(unit.ServiceName(), gc.Equals, service.Name())

	saved, err := s.State.Unit(unit.Name())
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(saved.Name(), gc.Equals, unit.Name())
	c.Assert(saved.ServiceName(), gc.Equals, unit.ServiceName())
	c.Assert(saved.Series(), gc.Equals, unit.Series())
	c.Assert(saved.Life(), gc.Equals, unit.Life())

	serviceCharmURL, _ := service.CharmURL()
	unitCharmURL, _ := saved.CharmURL()
	c.Assert(unitCharmURL, gc.DeepEquals, serviceCharmURL)
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
	s1 := s.Factory.MakeService(c, &factory.ServiceParams{
		Name: "service1",
		Charm: s.Factory.MakeCharm(c, &factory.CharmParams{
			Name: "wordpress",
		}),
	})
	e1, err := s1.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)

	s2 := s.Factory.MakeService(c, &factory.ServiceParams{
		Name: "service2",
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
	meteredService := s.Factory.MakeService(c, &factory.ServiceParams{Charm: meteredCharm})
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: meteredService, SetCharmURL: true})
	metric := s.Factory.MakeMetric(c, &factory.MetricParams{
		Unit:    unit,
		Time:    &now,
		Sent:    true,
		Metrics: []state.Metric{{"pings", "1", now}},
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

func (s *factorySuite) TestMakeEnvironmentNil(c *gc.C) {
	st := s.Factory.MakeEnvironment(c, nil)
	defer st.Close()

	env, err := st.Environment()
	c.Assert(err, jc.ErrorIsNil)
	re := regexp.MustCompile(`^testenv-\d+$`)
	c.Assert(re.MatchString(env.Name()), jc.IsTrue)
	c.Assert(env.UUID() == s.State.EnvironUUID(), jc.IsFalse)
	origEnv, err := s.State.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Owner(), gc.Equals, origEnv.Owner())

	cfg, err := st.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["default-series"], gc.Equals, "trusty")
}

func (s *factorySuite) TestMakeEnvironment(c *gc.C) {
	owner := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "owner",
	})
	params := &factory.EnvParams{
		Name:        "foo",
		Owner:       owner.UserTag(),
		ConfigAttrs: testing.Attrs{"default-series": "precise"},
	}

	st := s.Factory.MakeEnvironment(c, params)
	defer st.Close()

	env, err := st.Environment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.Name(), gc.Equals, "foo")
	c.Assert(env.UUID() == s.State.EnvironUUID(), jc.IsFalse)
	c.Assert(env.Owner(), gc.Equals, owner.UserTag())

	cfg, err := st.EnvironConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cfg.AllAttrs()["default-series"], gc.Equals, "precise")
}
