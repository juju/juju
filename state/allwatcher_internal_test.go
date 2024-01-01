// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"sort"
	"time"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/model"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/multiwatcher"
	"github.com/juju/juju/core/network"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testing"
)

const allEndpoints = ""

var (
	_ backingEntityDoc = (*backingMachine)(nil)
	_ backingEntityDoc = (*backingUnit)(nil)
	_ backingEntityDoc = (*backingApplication)(nil)
	_ backingEntityDoc = (*backingCharm)(nil)
	_ backingEntityDoc = (*backingRemoteApplication)(nil)
	_ backingEntityDoc = (*backingApplicationOffer)(nil)
	_ backingEntityDoc = (*backingRelation)(nil)
	_ backingEntityDoc = (*backingAnnotation)(nil)
	_ backingEntityDoc = (*backingStatus)(nil)
	_ backingEntityDoc = (*backingConstraints)(nil)
	_ backingEntityDoc = (*backingSettings)(nil)
	_ backingEntityDoc = (*backingOpenedPorts)(nil)
	_ backingEntityDoc = (*backingAction)(nil)
	_ backingEntityDoc = (*backingBlock)(nil)
	_ backingEntityDoc = (*backingGeneration)(nil)
	_ backingEntityDoc = (*backingPodSpec)(nil)
)

var dottedConfig = `
options:
  key.dotted: {default: My Key, description: Desc, type: string}
`

type allWatcherBaseSuite struct {
	internalStateSuite
	currentTime time.Time
}

func (s *allWatcherBaseSuite) SetUpTest(c *gc.C) {
	s.internalStateSuite.SetUpTest(c)
	s.currentTime = s.state.clock().Now()
	loggo.GetLogger("juju.state.allwatcher").SetLogLevel(loggo.TRACE)
}

// setUpScenario adds some entities to the state so that
// we can check that they all get pulled in by
// all(Model)WatcherStateBacking.GetAll.
func (s *allWatcherBaseSuite) setUpScenario(c *gc.C, st *State, units int) (entities entityInfoSlice) {
	modelUUID := st.ModelUUID()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	add := func(e multiwatcher.EntityInfo) {
		entities = append(entities, e)
	}

	modelCfg, err := model.Config()
	c.Assert(err, jc.ErrorIsNil)
	modelStatus, err := model.Status()
	c.Assert(err, jc.ErrorIsNil)
	credential, _ := model.CloudCredentialTag()
	add(&multiwatcher.ModelInfo{
		ModelUUID:       model.UUID(),
		Type:            coremodel.ModelType(model.Type()),
		Name:            model.Name(),
		Life:            life.Alive,
		Owner:           model.Owner().Id(),
		ControllerUUID:  model.ControllerUUID(),
		IsController:    model.IsControllerModel(),
		Cloud:           model.CloudName(),
		CloudRegion:     model.CloudRegion(),
		CloudCredential: credential.Id(),
		Config:          modelCfg.AllAttrs(),
		Status: multiwatcher.StatusInfo{
			Current: modelStatus.Status,
			Message: modelStatus.Message,
			Data:    modelStatus.Data,
			Since:   modelStatus.Since,
		},
		SLA: multiwatcher.ModelSLAInfo{
			Level: "unsupported",
		},
		UserPermissions: map[string]permission.Access{
			"test-admin": permission.AdminAccess,
		},
	})

	now := s.currentTime
	m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m.Tag(), gc.Equals, names.NewMachineTag("0"))
	// Ensure there's one and only one controller.
	controllerIds, err := st.ControllerIds()
	c.Assert(err, jc.ErrorIsNil)
	needController := len(controllerIds) == 0
	if needController {
		_, err = st.EnableHA(1, constraints.Value{}, UbuntuBase("20.04"), []string{m.Id()})
		c.Assert(err, jc.ErrorIsNil)
		node, err := st.ControllerNode(m.Id())
		c.Assert(err, jc.ErrorIsNil)
		err = node.SetHasVote(true)
		c.Assert(err, jc.ErrorIsNil)
	}
	// TODO(dfc) instance.Id should take a TAG!
	err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "", "fake_nonce", nil)
	c.Assert(err, jc.ErrorIsNil)
	hc, err := m.HardwareCharacteristics()
	c.Assert(err, jc.ErrorIsNil)
	cp, err := m.CharmProfiles()
	c.Assert(err, jc.ErrorIsNil)

	// Add a space and an address on the space.
	space, err := st.AddSpace("test-space", "provider-space", nil, true)
	c.Assert(err, jc.ErrorIsNil)
	providerAddr := network.NewSpaceAddress("example.com")
	providerAddr.SpaceID = space.Id()
	err = m.SetProviderAddresses(providerAddr)
	c.Assert(err, jc.ErrorIsNil)

	var addresses []network.ProviderAddress
	for _, addr := range m.Addresses() {
		addresses = append(addresses, network.ProviderAddress{
			MachineAddress:  addr.MachineAddress,
			SpaceName:       network.SpaceName(space.Name()),
			ProviderSpaceID: space.ProviderId(),
		})
	}

	jobs := []coremodel.MachineJob{JobHostUnits.ToParams()}
	if needController {
		jobs = append(jobs, JobManageModel.ToParams())
	}
	add(&multiwatcher.MachineInfo{
		ModelUUID:  modelUUID,
		ID:         "0",
		InstanceID: "i-machine-0",
		AgentStatus: multiwatcher.StatusInfo{
			Current: status.Pending,
			Data:    map[string]interface{}{},
			Since:   &now,
		},
		InstanceStatus: multiwatcher.StatusInfo{
			Current: status.Pending,
			Data:    map[string]interface{}{},
			Since:   &now,
		},
		Life:                    life.Alive,
		Base:                    "ubuntu@12.10",
		Jobs:                    jobs,
		Addresses:               addresses,
		HardwareCharacteristics: hc,
		CharmProfiles:           cp,
		HasVote:                 needController,
		WantsVote:               needController,
		PreferredPublicAddress:  providerAddr,
		PreferredPrivateAddress: providerAddr,
	})

	wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
	err = wordpress.MergeExposeSettings(nil)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.SetMinUnits(units)
	c.Assert(err, jc.ErrorIsNil)
	err = wordpress.SetConstraints(constraints.MustParse("mem=100M"))
	c.Assert(err, jc.ErrorIsNil)
	setApplicationConfigAttr(c, wordpress, "blog-title", "boring")
	pairs := map[string]string{"x": "12", "y": "99"}
	add(&multiwatcher.ApplicationInfo{
		ModelUUID:   modelUUID,
		Name:        "wordpress",
		Exposed:     true,
		CharmURL:    applicationCharmURL(wordpress).String(),
		Life:        life.Alive,
		MinUnits:    units,
		Constraints: constraints.MustParse("mem=100M"),
		Annotations: pairs,
		Config:      charm.Settings{"blog-title": "boring"},
		Subordinate: false,
		Status: multiwatcher.StatusInfo{
			Current: "unset",
			Message: "",
			Data:    map[string]interface{}{},
			Since:   &now,
		},
	})
	err = model.SetAnnotations(wordpress, pairs)
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.AnnotationInfo{
		ModelUUID:   modelUUID,
		Tag:         "application-wordpress",
		Annotations: pairs,
	})

	add(&multiwatcher.CharmInfo{
		ModelUUID:     modelUUID,
		CharmURL:      applicationCharmURL(wordpress).String(),
		Life:          life.Alive,
		DefaultConfig: map[string]interface{}{"blog-title": "My Title"},
	})

	logging := AddTestingApplication(c, st, "logging", AddTestingCharm(c, st, "logging"))
	add(&multiwatcher.ApplicationInfo{
		ModelUUID:   modelUUID,
		Name:        "logging",
		CharmURL:    applicationCharmURL(logging).String(),
		Life:        life.Alive,
		Config:      charm.Settings{},
		Subordinate: true,
		Status: multiwatcher.StatusInfo{
			Current: "unset",
			Message: "",
			Data:    map[string]interface{}{},
			Since:   &now,
		},
	})

	add(&multiwatcher.CharmInfo{
		ModelUUID: modelUUID,
		CharmURL:  applicationCharmURL(logging).String(),
		Life:      life.Alive,
	})

	eps, err := st.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.RelationInfo{
		ModelUUID: modelUUID,
		Key:       "logging:logging-directory wordpress:logging-dir",
		ID:        rel.Id(),
		Endpoints: []multiwatcher.Endpoint{
			{ApplicationName: "logging", Relation: multiwatcher.CharmRelation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}},
			{ApplicationName: "wordpress", Relation: multiwatcher.CharmRelation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
	})

	for i := 0; i < units; i++ {
		wu, err := wordpress.AddUnit(AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(wu.Tag().String(), gc.Equals, fmt.Sprintf("unit-wordpress-%d", i))

		m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(m.Tag().String(), gc.Equals, fmt.Sprintf("machine-%d", i+1))

		pairs := map[string]string{"name": fmt.Sprintf("bar %d", i)}
		add(&multiwatcher.UnitInfo{
			ModelUUID:   modelUUID,
			Name:        fmt.Sprintf("wordpress/%d", i),
			Application: wordpress.Name(),
			Base:        "ubuntu@12.10",
			Life:        life.Alive,
			MachineID:   m.Id(),
			Annotations: pairs,
			Subordinate: false,
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "waiting",
				Message: "waiting for machine",
				Data:    map[string]interface{}{},
				Since:   &now,
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Message: "",
				Data:    map[string]interface{}{},
				Since:   &now,
			},
		})
		err = model.SetAnnotations(wu, pairs)
		c.Assert(err, jc.ErrorIsNil)
		add(&multiwatcher.AnnotationInfo{
			ModelUUID:   modelUUID,
			Tag:         fmt.Sprintf("unit-wordpress-%d", i),
			Annotations: pairs,
		})
		err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "", "fake_nonce", nil)
		c.Assert(err, jc.ErrorIsNil)
		sInfo := status.StatusInfo{
			Status:  status.Error,
			Message: m.Tag().String(),
			Since:   &now,
		}
		err = m.SetStatus(sInfo)
		c.Assert(err, jc.ErrorIsNil)
		hc, err := m.HardwareCharacteristics()
		c.Assert(err, jc.ErrorIsNil)
		cp, err := m.CharmProfiles()
		c.Assert(err, jc.ErrorIsNil)
		add(&multiwatcher.MachineInfo{
			ModelUUID:  modelUUID,
			ID:         fmt.Sprint(i + 1),
			InstanceID: "i-" + m.Tag().String(),
			AgentStatus: multiwatcher.StatusInfo{
				Current: status.Error,
				Message: m.Tag().String(),
				Data:    map[string]interface{}{},
				Since:   &now,
			},
			InstanceStatus: multiwatcher.StatusInfo{
				Current: status.Pending,
				Data:    map[string]interface{}{},
				Since:   &now,
			},
			Life:                    life.Alive,
			Base:                    "ubuntu@12.10",
			Jobs:                    []coremodel.MachineJob{JobHostUnits.ToParams()},
			Addresses:               []network.ProviderAddress{},
			HardwareCharacteristics: hc,
			CharmProfiles:           cp,
			HasVote:                 false,
			WantsVote:               false,
		})
		err = wu.AssignToMachine(m)
		c.Assert(err, jc.ErrorIsNil)

		wru, err := rel.Unit(wu)
		c.Assert(err, jc.ErrorIsNil)

		// Create the subordinate unit as a side-effect of entering
		// scope in the principal's relation-unit.
		err = wru.EnterScope(nil)
		c.Assert(err, jc.ErrorIsNil)

		lu, err := st.Unit(fmt.Sprintf("logging/%d", i))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(lu.IsPrincipal(), jc.IsFalse)
		unitName := fmt.Sprintf("wordpress/%d", i)
		add(&multiwatcher.UnitInfo{
			ModelUUID:   modelUUID,
			Name:        fmt.Sprintf("logging/%d", i),
			Application: "logging",
			Base:        "ubuntu@12.10",
			Life:        life.Alive,
			MachineID:   m.Id(),
			Principal:   unitName,
			Subordinate: true,
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "waiting",
				Message: "waiting for machine",
				Data:    map[string]interface{}{},
				Since:   &now,
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Message: "",
				Data:    map[string]interface{}{},
				Since:   &now,
			},
		})
	}

	_, remoteApplicationInfo := addTestingRemoteApplication(
		c, st, "remote-mysql1", "me/model.mysql", mysqlRelations, false,
	)
	add(&remoteApplicationInfo)

	mysql := AddTestingApplication(c, st, "mysql", AddTestingCharm(c, st, "mysql"))
	curl := applicationCharmURL(mysql)
	add(&multiwatcher.ApplicationInfo{
		ModelUUID:   modelUUID,
		Name:        "mysql",
		CharmURL:    curl.String(),
		Life:        life.Alive,
		Config:      charm.Settings{},
		Constraints: constraints.MustParse("arch=amd64"),
		Status: multiwatcher.StatusInfo{
			Current: "unset",
			Message: "",
			Data:    map[string]interface{}{},
			Since:   &now,
		},
	})

	add(&multiwatcher.CharmInfo{
		ModelUUID:     modelUUID,
		CharmURL:      applicationCharmURL(mysql).String(),
		Life:          life.Alive,
		DefaultConfig: map[string]interface{}{"dataset-size": "80%"},
	})

	// Set up a remote application related to the offer.
	// It won't be included in the backing model.
	addTestingRemoteApplication(
		c, st, "remote-wordpress", "", []charm.Relation{{
			Name:      "db",
			Role:      "requirer",
			Scope:     charm.ScopeGlobal,
			Interface: "mysql",
		}}, true,
	)
	addTestingRemoteApplication(
		c, st, "remote-wordpress2", "", []charm.Relation{{
			Name:      "db",
			Role:      "requirer",
			Scope:     charm.ScopeGlobal,
			Interface: "mysql",
		}}, true,
	)
	eps, err = st.InferEndpoints("mysql", "remote-wordpress2")
	c.Assert(err, jc.ErrorIsNil)
	rel, err = st.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	add(&multiwatcher.RelationInfo{
		ModelUUID: modelUUID,
		Key:       rel.Tag().Id(),
		ID:        rel.Id(),
		Endpoints: []multiwatcher.Endpoint{
			{ApplicationName: "mysql", Relation: multiwatcher.CharmRelation{Name: "server", Role: "provider", Interface: "mysql", Optional: false, Limit: 0, Scope: "global"}},
			{ApplicationName: "remote-wordpress2", Relation: multiwatcher.CharmRelation{Name: "db", Role: "requirer", Interface: "mysql", Optional: false, Limit: 0, Scope: "global"}}},
	})

	applicationOfferInfo, rel2 := addTestingApplicationOffer(
		c, st, s.owner, "hosted-mysql", "mysql", curl.Name, []string{"server"},
	)
	add(&multiwatcher.RelationInfo{
		ModelUUID: modelUUID,
		Key:       rel2.Tag().Id(),
		ID:        rel2.Id(),
		Endpoints: []multiwatcher.Endpoint{
			{ApplicationName: "mysql", Relation: multiwatcher.CharmRelation{Name: "server", Role: "provider", Interface: "mysql", Optional: false, Limit: 0, Scope: "global"}},
			{ApplicationName: "remote-wordpress", Relation: multiwatcher.CharmRelation{Name: "db", Role: "requirer", Interface: "mysql", Optional: false, Limit: 0, Scope: "global"}}},
	})
	add(&applicationOfferInfo)

	return
}

var mysqlRelations = []charm.Relation{{
	Name:      "db",
	Role:      "provider",
	Scope:     charm.ScopeGlobal,
	Interface: "mysql",
}, {
	Name:      "nrpe-external-master",
	Role:      "provider",
	Scope:     charm.ScopeGlobal,
	Interface: "nrpe-external-master",
}}

func addTestingRemoteApplication(
	c *gc.C, st *State, name, url string, relations []charm.Relation, isProxy bool,
) (*RemoteApplication, multiwatcher.RemoteApplicationUpdate) {

	rs, err := st.AddRemoteApplication(AddRemoteApplicationParams{
		Name:            name,
		URL:             url,
		SourceModel:     testing.ModelTag,
		Endpoints:       relations,
		IsConsumerProxy: isProxy,
	})
	c.Assert(err, jc.ErrorIsNil)
	var appStatus multiwatcher.StatusInfo
	if !isProxy {
		status, err := rs.Status()
		c.Assert(err, jc.ErrorIsNil)
		appStatus = multiwatcher.StatusInfo{
			Current: status.Status,
			Message: status.Message,
			Data:    status.Data,
			Since:   status.Since,
		}
	}
	return rs, multiwatcher.RemoteApplicationUpdate{
		ModelUUID: st.ModelUUID(),
		Name:      name,
		OfferURL:  url,
		Life:      life.Value(rs.Life().String()),
		Status:    appStatus,
	}
}

func addTestingApplicationOffer(
	c *gc.C, st *State, owner names.UserTag, offerName, applicationName, charmName string, endpoints []string,
) (multiwatcher.ApplicationOfferInfo, *Relation) {

	eps := make(map[string]string)
	for _, ep := range endpoints {
		eps[ep] = ep
	}
	offers := NewApplicationOffers(st)
	offer, err := offers.AddOffer(crossmodel.AddApplicationOfferArgs{
		OfferName:       offerName,
		Owner:           owner.Name(),
		ApplicationName: applicationName,
		Endpoints:       eps,
	})
	c.Assert(err, jc.ErrorIsNil)
	relEps, err := st.InferEndpoints("mysql", "remote-wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := st.AddRelation(relEps...)
	c.Assert(err, jc.ErrorIsNil)
	err = rel.SetStatus(status.StatusInfo{Status: status.Joined})
	c.Assert(err, jc.ErrorIsNil)
	_, err = st.AddOfferConnection(AddOfferConnectionParams{
		SourceModelUUID: utils.MustNewUUID().String(),
		RelationId:      rel.Id(),
		Username:        "fred",
		OfferUUID:       offer.OfferUUID,
		RelationKey:     rel.Tag().Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	return multiwatcher.ApplicationOfferInfo{
		ModelUUID:            st.ModelUUID(),
		OfferName:            offerName,
		OfferUUID:            offer.OfferUUID,
		ApplicationName:      applicationName,
		CharmName:            charmName,
		TotalConnectedCount:  1,
		ActiveConnectedCount: 1,
	}, rel
}

var _ = gc.Suite(&allWatcherStateSuite{})

type allWatcherStateSuite struct {
	allWatcherBaseSuite
}

func (s *allWatcherStateSuite) reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func (s *allWatcherStateSuite) TestGetAll(c *gc.C) {
	expectEntities := s.setUpScenario(c, s.state, 2)
	s.checkGetAll(c, expectEntities)
}

func (s *allWatcherStateSuite) TestGetAllMultiModel(c *gc.C) {
	// Set up 2 models and ensure that GetAll returns the
	// entities for both models with no errors.
	expectEntities := s.setUpScenario(c, s.state, 2)

	// Use more units in the second model.
	moreEntities := s.setUpScenario(c, s.newState(c), 4)

	expectEntities = append(expectEntities, moreEntities...)

	s.checkGetAll(c, expectEntities)
}

func (s *allWatcherStateSuite) checkGetAll(c *gc.C, expectEntities entityInfoSlice) {
	b, err := NewAllWatcherBacking(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	all := multiwatcher.NewStore(loggo.GetLogger("test"))
	err = b.GetAll(all)
	c.Assert(err, jc.ErrorIsNil)
	var gotEntities entityInfoSlice = all.All()
	sort.Sort(gotEntities)
	sort.Sort(expectEntities)
	assertEntitiesEqual(c, gotEntities, expectEntities)
}

func applicationCharmURL(app *Application) *charm.URL {
	urlStr, _ := app.CharmURL()
	url := charm.MustParseURL(*urlStr)
	return url
}

func setApplicationConfigAttr(c *gc.C, app *Application, attr string, val interface{}) {
	err := app.UpdateCharmConfig(model.GenerationMaster, charm.Settings{attr: val})
	c.Assert(err, jc.ErrorIsNil)
}

func setModelConfigAttr(c *gc.C, st *State, attr string, val interface{}) {
	m, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	err = m.UpdateModelConfig(map[string]interface{}{attr: val}, nil)
	c.Assert(err, jc.ErrorIsNil)
}

// changeTestCase encapsulates entities to add, a change, and
// the expected contents for a test.
type changeTestCase struct {
	// about describes the test case.
	about string

	// initialContents contains the infos of the
	// watcher before signaling the change.
	initialContents []multiwatcher.EntityInfo

	// change signals the change of the watcher.
	change watcher.Change

	// expectContents contains the expected infos of
	// the watcher after signaling the change.
	expectContents []multiwatcher.EntityInfo
}

// changeTestFunc is a function for the preparation of a test and
// the creation of the according case.
type changeTestFunc func(c *gc.C, st *State) changeTestCase

// performChangeTestCases runs a passed number of test cases for changes.
func (s *allWatcherStateSuite) performChangeTestCases(c *gc.C, changeTestFuncs []changeTestFunc) {
	for i, changeTestFunc := range changeTestFuncs {
		test := changeTestFunc(c, s.state)

		c.Logf("test %d. %s", i, test.about)
		b, err := NewAllWatcherBacking(s.pool)
		c.Assert(err, jc.ErrorIsNil)
		all := multiwatcher.NewStore(loggo.GetLogger("test"))
		for _, info := range test.initialContents {
			all.Update(info)
		}
		err = b.Changed(all, test.change)
		c.Assert(err, jc.ErrorIsNil)
		entities := all.All()
		assertEntitiesEqual(c, entities, test.expectContents)
		s.reset(c)
	}
}

func (s *allWatcherStateSuite) TestChangePermissions(c *gc.C) {
	testChangePermissions(c, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeAnnotations(c *gc.C) {
	testChangeAnnotations(c, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeMachines(c *gc.C) {
	testChangeMachines(c, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeRelations(c *gc.C) {
	testChangeRelations(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeApplications(c *gc.C) {
	testChangeApplications(c, s.owner, s.performChangeTestCases)
}

func strPtr(s string) *string {
	return &s
}

func (s *allWatcherStateSuite) TestChangeCAASApplications(c *gc.C) {
	loggo.GetLogger("juju.txn").SetLogLevel(loggo.TRACE)
	changeTestFuncs := []changeTestFunc{
		// Applications.
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "not finding a podspec for a change is fine",
				change: watcher.Change{
					C:  "podSpecs",
					Id: st.docID(applicationGlobalKey("mysql")),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			caasSt := s.newCAASState(c)
			m, err := caasSt.Model()
			c.Assert(err, jc.ErrorIsNil)
			cm, err := m.CAASModel()
			c.Assert(err, jc.ErrorIsNil)
			ch := AddTestingCharmForSeries(c, caasSt, "kubernetes", "mysql")
			mysql := AddTestingApplicationForBase(c, caasSt, UbuntuBase("20.04"), "mysql", ch)
			err = cm.SetPodSpec(nil, mysql.ApplicationTag(), strPtr("some podspec"))
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			return changeTestCase{
				about: "initial CAAS application has podspec",
				change: watcher.Change{
					C:  "applications",
					Id: caasSt.docID("mysql"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID:   caasSt.ModelUUID(),
						Name:        "mysql",
						CharmURL:    "local:kubernetes/kubernetes-mysql-0",
						Life:        "alive",
						Config:      map[string]interface{}{},
						Constraints: constraints.MustParse("arch=amd64"),
						Status: multiwatcher.StatusInfo{
							Current: "unset",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						OperatorStatus: multiwatcher.StatusInfo{
							Current: "waiting",
							Message: "waiting for container",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						PodSpec: &multiwatcher.PodSpec{
							Spec: "some podspec",
						},
					},
				},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			caasSt := s.newCAASState(c)
			m, err := caasSt.Model()
			c.Assert(err, jc.ErrorIsNil)
			cm, err := m.CAASModel()
			c.Assert(err, jc.ErrorIsNil)
			ch := AddTestingCharmForSeries(c, caasSt, "kubernetes", "mysql")
			mysql := AddTestingApplicationForBase(c, caasSt, UbuntuBase("20.04"), "mysql", ch)
			err = cm.SetPodSpec(nil, mysql.ApplicationTag(), strPtr("some podspec"))
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "application podspec is updated",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: caasSt.ModelUUID(),
						Name:      "mysql",
					},
				},
				change: watcher.Change{
					C:  "podSpecs",
					Id: caasSt.docID(applicationGlobalKey("mysql")),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: caasSt.ModelUUID(),
						Name:      "mysql",
						PodSpec: &multiwatcher.PodSpec{
							Spec: "some podspec",
						},
					},
				},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			caasSt := s.newCAASState(c)
			ch := AddTestingCharmForSeries(c, caasSt, "kubernetes", "mysql")
			mysql := AddTestingApplicationForBase(c, caasSt, UbuntuBase("20.04"), "mysql", ch)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Error,
				Message: "failure",
				Since:   &now,
			}
			err := mysql.SetOperatorStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "operator status update, updates application",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: caasSt.ModelUUID(),
						Name:      "mysql",
					},
				},
				change: watcher.Change{
					C:  "statuses",
					Id: caasSt.docID(applicationGlobalOperatorKey("mysql")),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: caasSt.ModelUUID(),
						Name:      "mysql",
						OperatorStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					},
				},
			}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

func (s *allWatcherStateSuite) TestChangeCAASUnits(c *gc.C) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			caasSt := s.newCAASState(c)
			ch := AddTestingCharmForSeries(c, caasSt, "kubernetes", "mysql")
			mysql := AddTestingApplicationForBase(c, caasSt, UbuntuBase("20.04"), "mysql", ch)
			unit, err := mysql.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)

			updateUnits := UpdateUnitsOperation{
				Updates: []*UpdateUnitOperation{
					unit.UpdateOperation(UnitUpdateProperties{
						CloudContainerStatus: &status.StatusInfo{Status: status.Maintenance, Message: "setting up"},
					}),
				},
			}
			err = mysql.UpdateUnits(&updateUnits)
			c.Assert(err, jc.ErrorIsNil)

			now := st.clock().Now()
			return changeTestCase{
				about: "initial CAAS unit has container status",
				change: watcher.Change{
					C:  "units",
					Id: caasSt.docID("mysql/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   caasSt.ModelUUID(),
						Name:        "mysql/0",
						Application: "mysql",
						Base:        "ubuntu@20.04",
						Life:        "alive",
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "waiting",
							Message: "installing agent",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						ContainerStatus: multiwatcher.StatusInfo{
							Current: "maintenance",
							Message: "setting up",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					},
				},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			caasSt := s.newCAASState(c)
			ch := AddTestingCharmForSeries(c, caasSt, "kubernetes", "mysql")
			mysql := AddTestingApplicationForBase(c, caasSt, UbuntuBase("20.04"), "mysql", ch)
			unit, err := mysql.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)

			updateUnits := UpdateUnitsOperation{
				Updates: []*UpdateUnitOperation{
					unit.UpdateOperation(UnitUpdateProperties{
						CloudContainerStatus: &status.StatusInfo{Status: status.Maintenance, Message: "setting up"},
					}),
				},
			}
			err = mysql.UpdateUnits(&updateUnits)
			c.Assert(err, jc.ErrorIsNil)

			now := st.clock().Now()
			return changeTestCase{
				about: "container status updates existing unit",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   caasSt.ModelUUID(),
						Name:        "mysql/0",
						Application: "mysql",
						Base:        "ubuntu@20.04",
					},
				},
				change: watcher.Change{
					C:  "statuses",
					Id: caasSt.docID(unit.globalCloudContainerKey()),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   caasSt.ModelUUID(),
						Name:        "mysql/0",
						Application: "mysql",
						Base:        "ubuntu@20.04",
						ContainerStatus: multiwatcher.StatusInfo{
							Current: "maintenance",
							Message: "setting up",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					},
				},
			}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

func (s *allWatcherStateSuite) TestChangeCharms(c *gc.C) {
	testChangeCharms(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeApplicationsConstraints(c *gc.C) {
	testChangeApplicationsConstraints(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeUnits(c *gc.C) {
	testChangeUnits(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeUnitsNonNilPorts(c *gc.C) {
	testChangeUnitsNonNilPorts(c, s.owner, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeRemoteApplications(c *gc.C) {
	testChangeRemoteApplications(c, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeApplicationOffers(c *gc.C) {
	testChangeApplicationOffers(c, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeGenerations(c *gc.C) {
	testChangeGenerations(c, s.performChangeTestCases)
}

func (s *allWatcherStateSuite) TestChangeActions(c *gc.C) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			operationID, err := m.EnqueueOperation("a test", 1)
			c.Assert(err, jc.ErrorIsNil)
			action, err := m.EnqueueAction(operationID, u.Tag(), "vacuumdb", map[string]interface{}{}, true, "group", nil)
			c.Assert(err, jc.ErrorIsNil)
			enqueued := makeActionInfo(action, st)
			action, err = action.Begin()
			c.Assert(err, jc.ErrorIsNil)
			started := makeActionInfo(action, st)
			return changeTestCase{
				about:           "action change picks up last change",
				initialContents: []multiwatcher.EntityInfo{&enqueued, &started},
				change:          watcher.Change{C: actionsC, Id: st.docID(action.Id())},
				expectContents:  []multiwatcher.EntityInfo{&started},
			}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

func (s *allWatcherStateSuite) TestChangeBlocks(c *gc.C) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no blocks in state, no blocks in store -> do nothing",
				change: watcher.Change{
					C:  blocksC,
					Id: st.docID("1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			blockId := st.docID("0")
			blockType := DestroyBlock.ToParams()
			blockMsg := "woot"
			m, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "no change if block is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.BlockInfo{
					ModelUUID: st.ModelUUID(),
					ID:        blockId,
					Type:      blockType,
					Message:   blockMsg,
					Tag:       m.ModelTag().String(),
				}},
				change: watcher.Change{
					C:  blocksC,
					Id: blockId,
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.BlockInfo{
					ModelUUID: st.ModelUUID(),
					ID:        blockId,
					Type:      blockType,
					Message:   blockMsg,
					Tag:       m.ModelTag().String(),
				}},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			err := st.SwitchBlockOn(DestroyBlock, "multiwatcher testing")
			c.Assert(err, jc.ErrorIsNil)
			b, found, err := st.GetBlockForType(DestroyBlock)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(found, jc.IsTrue)
			blockId := b.Id()
			m, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "block is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  blocksC,
					Id: blockId,
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.BlockInfo{
						ModelUUID: st.ModelUUID(),
						ID:        st.localID(blockId),
						Type:      b.Type().ToParams(),
						Message:   b.Message(),
						Tag:       m.ModelTag().String(),
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			err := st.SwitchBlockOn(DestroyBlock, "multiwatcher testing")
			c.Assert(err, jc.ErrorIsNil)
			b, found, err := st.GetBlockForType(DestroyBlock)
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(found, jc.IsTrue)
			err = st.SwitchBlockOff(DestroyBlock)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "block is removed if it's in backing and in multiwatcher.Store",
				change: watcher.Change{
					C:  blocksC,
					Id: b.Id(),
				},
			}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

func (s *allWatcherStateSuite) TestClosingPorts(c *gc.C) {
	// Init the test model.
	wordpress := AddTestingApplication(c, s.state, "wordpress", AddTestingCharm(c, s.state, "wordpress"))
	u, err := wordpress.AddUnit(AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	m, err := s.state.AddMachine(UbuntuBase("12.10"), JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	err = u.AssignToMachine(m)
	c.Assert(err, jc.ErrorIsNil)
	publicAddress := network.NewSpaceAddress("1.2.3.4", network.WithScope(network.ScopePublic))
	privateAddress := network.NewSpaceAddress("4.3.2.1", network.WithScope(network.ScopeCloudLocal))
	MustOpenUnitPortRange(c, s.state, m, u.Name(), allEndpoints, corenetwork.MustParsePortRange("12345/tcp"))
	// Create all watcher state backing.
	b, err := NewAllWatcherBacking(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	all := multiwatcher.NewStore(loggo.GetLogger("test"))
	machineInfo := &multiwatcher.MachineInfo{
		ModelUUID:               s.state.ModelUUID(),
		ID:                      "0",
		PreferredPublicAddress:  publicAddress,
		PreferredPrivateAddress: privateAddress,
	}
	all.Update(machineInfo)
	// Check opened ports.
	err = b.Changed(all, watcher.Change{
		C:  "units",
		Id: s.state.docID("wordpress/0"),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities := all.All()
	now := s.currentTime
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{
		&multiwatcher.UnitInfo{
			ModelUUID:      s.state.ModelUUID(),
			Name:           "wordpress/0",
			Application:    "wordpress",
			Base:           "ubuntu@12.10",
			Life:           life.Alive,
			MachineID:      "0",
			PublicAddress:  "1.2.3.4",
			PrivateAddress: "4.3.2.1",
			OpenPortRangesByEndpoint: corenetwork.GroupedPortRanges{
				allEndpoints: {corenetwork.MustParsePortRange("12345/tcp")},
			},
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "waiting",
				Message: "waiting for machine",
				Data:    map[string]interface{}{},
				Since:   &now,
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Data:    map[string]interface{}{},
				Since:   &now,
			},
		},
		machineInfo,
	})
	// Close the ports.
	MustCloseUnitPortRange(c, s.state, m, u.Name(), allEndpoints, corenetwork.MustParsePortRange("12345/tcp"))
	err = b.Changed(all, watcher.Change{
		C:  openedPortsC,
		Id: s.state.docID("0"),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities = all.All()
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{
		&multiwatcher.UnitInfo{
			ModelUUID:      s.state.ModelUUID(),
			Name:           "wordpress/0",
			Application:    "wordpress",
			Base:           "ubuntu@12.10",
			MachineID:      "0",
			Life:           life.Alive,
			PublicAddress:  "1.2.3.4",
			PrivateAddress: "4.3.2.1",
			WorkloadStatus: multiwatcher.StatusInfo{
				Current: "waiting",
				Message: "waiting for machine",
				Data:    map[string]interface{}{},
				Since:   &now,
			},
			AgentStatus: multiwatcher.StatusInfo{
				Current: "allocating",
				Data:    map[string]interface{}{},
				Since:   &now,
			},
		},
		machineInfo,
	})
}

func (s *allWatcherStateSuite) TestApplicationSettings(c *gc.C) {
	// Init the test model.
	app := AddTestingApplication(c, s.state, "dummy-application", AddTestingCharm(c, s.state, "dummy"))
	b, err := NewAllWatcherBacking(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	all := multiwatcher.NewStore(loggo.GetLogger("test"))
	// 1st scenario part: set settings and signal change.
	setApplicationConfigAttr(c, app, "username", "foo")
	setApplicationConfigAttr(c, app, "outlook", "foo@bar")
	all.Update(&multiwatcher.ApplicationInfo{
		ModelUUID: s.state.ModelUUID(),
		Name:      "dummy-application",
		CharmURL:  "local:quantal/quantal-dummy-1",
	})
	err = b.Changed(all, watcher.Change{
		C:  "settings",
		Id: s.state.docID("a#dummy-application#local:quantal/quantal-dummy-1"),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities := all.All()
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{
		&multiwatcher.ApplicationInfo{
			ModelUUID: s.state.ModelUUID(),
			Name:      "dummy-application",
			CharmURL:  "local:quantal/quantal-dummy-1",
			Config:    charm.Settings{"outlook": "foo@bar", "username": "foo"},
		},
	})
	// 2nd scenario part: destroy the application and signal change.
	err = app.Destroy()
	c.Assert(err, jc.ErrorIsNil)
	err = b.Changed(all, watcher.Change{
		C:  "applications",
		Id: s.state.docID("dummy-application"),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities = all.All()
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{})
}

var _ = gc.Suite(&allModelWatcherStateSuite{})

type allModelWatcherStateSuite struct {
	allWatcherBaseSuite
	state1 *State
}

func (s *allModelWatcherStateSuite) SetUpTest(c *gc.C) {
	s.allWatcherBaseSuite.SetUpTest(c)
	s.state1 = s.newState(c)
}

func (s *allModelWatcherStateSuite) Reset(c *gc.C) {
	s.TearDownTest(c)
	s.SetUpTest(c)
}

func (s *allModelWatcherStateSuite) NewAllWatcherBacking() (AllWatcherBacking, error) {
	return NewAllWatcherBacking(s.pool)
}

func (s *allModelWatcherStateSuite) TestMissingModelNotError(c *gc.C) {
	b, err := s.NewAllWatcherBacking()
	c.Assert(err, jc.ErrorIsNil)
	all := multiwatcher.NewStore(loggo.GetLogger("test"))

	dyingModel := "fake-uuid"
	st, err := s.pool.Get(dyingModel)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()

	removed, err := s.pool.Remove(dyingModel)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(removed, jc.IsFalse)

	// If the state pool is in the process of removing a model, it will
	// return a NotFound error.
	err = b.Changed(all, watcher.Change{C: modelsC, Id: dyingModel})
	c.Assert(err, jc.ErrorIsNil)
}

// performChangeTestCases runs a passed number of test cases for changes.
func (s *allModelWatcherStateSuite) performChangeTestCases(c *gc.C, changeTestFuncs []changeTestFunc) {
	for i, changeTestFunc := range changeTestFuncs {
		func() { // in aid of per-loop defers
			defer s.Reset(c)

			test0 := changeTestFunc(c, s.state)

			c.Logf("test %d. %s", i, test0.about)
			b, err := s.NewAllWatcherBacking()
			c.Assert(err, jc.ErrorIsNil)
			all := multiwatcher.NewStore(loggo.GetLogger("test"))

			// Do updates and check for first model.
			for _, info := range test0.initialContents {
				all.Update(info)
			}
			err = b.Changed(all, test0.change)
			c.Assert(err, jc.ErrorIsNil)
			var entities entityInfoSlice = all.All()
			assertEntitiesEqual(c, entities, test0.expectContents)

			// Now do the same updates for a second model.
			test1 := changeTestFunc(c, s.state1)
			for _, info := range test1.initialContents {
				all.Update(info)
			}
			err = b.Changed(all, test1.change)
			c.Assert(err, jc.ErrorIsNil)

			entities = all.All()

			// Expected to see entities for both models.
			var expectedEntities entityInfoSlice = append(
				test0.expectContents,
				test1.expectContents...)
			sort.Sort(entities)
			sort.Sort(expectedEntities)

			assertEntitiesEqual(c, entities, expectedEntities)
		}()
	}
}

func (s *allModelWatcherStateSuite) TestChangePermissions(c *gc.C) {
	testChangePermissions(c, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeAnnotations(c *gc.C) {
	testChangeAnnotations(c, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeMachines(c *gc.C) {
	testChangeMachines(c, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeRelations(c *gc.C) {
	testChangeRelations(c, s.owner, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeApplications(c *gc.C) {
	testChangeApplications(c, s.owner, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeApplicationsConstraints(c *gc.C) {
	testChangeApplicationsConstraints(c, s.owner, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeUnits(c *gc.C) {
	testChangeUnits(c, s.owner, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeUnitsNonNilPorts(c *gc.C) {
	testChangeUnitsNonNilPorts(c, s.owner, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeRemoteApplications(c *gc.C) {
	testChangeRemoteApplications(c, s.performChangeTestCases)
}

func (s *allModelWatcherStateSuite) TestChangeModels(c *gc.C) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no model in state -> do nothing",
				change: watcher.Change{
					C:  "models",
					Id: "non-existing-uuid",
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "model is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID: "some-uuid",
				}},
				change: watcher.Change{
					C:  "models",
					Id: "some-uuid",
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			model, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			err = model.SetSLA("essential", "test-sla-owner", nil)
			c.Assert(err, jc.ErrorIsNil)
			cfg, err := model.Config()
			c.Assert(err, jc.ErrorIsNil)
			status, err := model.Status()
			c.Assert(err, jc.ErrorIsNil)
			cons := constraints.MustParse("mem=4G")
			err = st.SetModelConstraints(cons)
			c.Assert(err, jc.ErrorIsNil)
			credential, _ := model.CloudCredentialTag()

			return changeTestCase{
				about: "model is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "models",
					Id: st.ModelUUID(),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ModelInfo{
						ModelUUID:       model.UUID(),
						Type:            coremodel.ModelType(model.Type()),
						Name:            model.Name(),
						Life:            life.Alive,
						Owner:           model.Owner().Id(),
						ControllerUUID:  model.ControllerUUID(),
						IsController:    model.IsControllerModel(),
						Cloud:           model.CloudName(),
						CloudRegion:     model.CloudRegion(),
						CloudCredential: credential.Id(),
						Config:          cfg.AllAttrs(),
						Constraints:     cons,
						Status: multiwatcher.StatusInfo{
							Current: status.Status,
							Message: status.Message,
							Data:    status.Data,
							Since:   status.Since,
						},
						SLA: multiwatcher.ModelSLAInfo{
							Level: "essential",
							Owner: "test-sla-owner",
						},
						UserPermissions: map[string]permission.Access{
							"test-admin": permission.AdminAccess,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			model, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			cfg, err := model.Config()
			c.Assert(err, jc.ErrorIsNil)
			status, err := model.Status()
			c.Assert(err, jc.ErrorIsNil)
			credential, _ := model.CloudCredentialTag()
			return changeTestCase{
				about: "model is updated if it's in backing and in Store",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.ModelInfo{
						ModelUUID:      model.UUID(),
						Name:           "",
						Life:           life.Alive,
						Owner:          model.Owner().Id(),
						ControllerUUID: model.ControllerUUID(),
						IsController:   model.IsControllerModel(),
						Config:         cfg.AllAttrs(),
						Status: multiwatcher.StatusInfo{
							Current: status.Status,
							Message: status.Message,
							Data:    status.Data,
							Since:   status.Since,
						},
						SLA: multiwatcher.ModelSLAInfo{
							Level: "unsupported",
						},
						UserPermissions: map[string]permission.Access{
							"test-admin": permission.AdminAccess,
						},
					},
				},
				change: watcher.Change{
					C:  "models",
					Id: model.UUID(),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ModelInfo{
						ModelUUID:       model.UUID(),
						Type:            coremodel.ModelType(model.Type()),
						Name:            model.Name(),
						Life:            life.Alive,
						Owner:           model.Owner().Id(),
						ControllerUUID:  model.ControllerUUID(),
						IsController:    model.IsControllerModel(),
						Cloud:           model.CloudName(),
						CloudRegion:     model.CloudRegion(),
						CloudCredential: credential.Id(),
						Config:          cfg.AllAttrs(),
						Status: multiwatcher.StatusInfo{
							Current: status.Status,
							Message: status.Message,
							Data:    status.Data,
							Since:   status.Since,
						},
						SLA: multiwatcher.ModelSLAInfo{
							Level: "unsupported",
						},
						UserPermissions: map[string]permission.Access{
							"test-admin": permission.AdminAccess,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			err := app.SetConstraints(constraints.MustParse("mem=4G arch=amd64"))
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the application exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M cores=2 cpu-power=4"),
				}},
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("a#wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress",
						Constraints: constraints.MustParse("mem=4G arch=amd64"),
					}}}
		},
	}
	s.performChangeTestCases(c, changeTestFuncs)
}

func (s *allModelWatcherStateSuite) TestChangeForDeadModel(c *gc.C) {
	// Ensure an entity is removed when a change is seen but
	// the model the entity belonged to has already died.

	b, err := NewAllWatcherBacking(s.pool)
	c.Assert(err, jc.ErrorIsNil)
	all := multiwatcher.NewStore(loggo.GetLogger("test"))

	// Insert a machine for an model that doesn't actually
	// exist (mimics model removal).
	all.Update(&multiwatcher.MachineInfo{
		ModelUUID: "uuid",
		ID:        "0",
	})
	c.Assert(all.All(), gc.HasLen, 1)

	err = b.Changed(all, watcher.Change{
		C:  "machines",
		Id: ensureModelUUID("uuid", "0"),
	})
	c.Assert(err, jc.ErrorIsNil)

	// Entity info should be gone now.
	c.Assert(all.All(), gc.HasLen, 0)
}

func (s *allModelWatcherStateSuite) TestModelSettings(c *gc.C) {
	// Init the test model.
	b, err := s.NewAllWatcherBacking()
	c.Assert(err, jc.ErrorIsNil)
	all := multiwatcher.NewStore(loggo.GetLogger("test"))
	setModelConfigAttr(c, s.state, "http-proxy", "http://invalid")
	setModelConfigAttr(c, s.state, "foo", "bar")

	m, err := s.state.Model()
	c.Assert(err, jc.ErrorIsNil)

	cfg, err := m.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	expectedModelSettings := cfg.AllAttrs()
	expectedModelSettings["http-proxy"] = "http://invalid"
	expectedModelSettings["foo"] = "bar"

	all.Update(&multiwatcher.ModelInfo{
		ModelUUID: s.state.ModelUUID(),
		Name:      "dummy-model",
	})
	err = b.Changed(all, watcher.Change{
		C:  "settings",
		Id: s.state.docID(modelGlobalKey),
	})
	c.Assert(err, jc.ErrorIsNil)
	entities := all.All()
	assertEntitiesEqual(c, entities, []multiwatcher.EntityInfo{
		&multiwatcher.ModelInfo{
			ModelUUID: s.state.ModelUUID(),
			Name:      "dummy-model",
			Config:    expectedModelSettings,
		},
	})
}

// The testChange* funcs are extracted so the test cases can be used
// to test both the allWatcher and allModelWatcher.

func testChangePermissions(c *gc.C, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			model, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			credential, _ := model.CloudCredentialTag()

			return changeTestCase{
				about: "model update keeps permissions",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID: st.ModelUUID(),
					// Existence doesn't care about the other values, and they are
					// not entirely relevant to this test.
					UserPermissions: map[string]permission.Access{
						"bob":  permission.ReadAccess,
						"mary": permission.AdminAccess,
					},
				}},
				change: watcher.Change{
					C:  "models",
					Id: st.ModelUUID(),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID:       st.ModelUUID(),
					Type:            coremodel.ModelType(model.Type()),
					Name:            model.Name(),
					Life:            "alive",
					Owner:           model.Owner().Id(),
					ControllerUUID:  testing.ControllerTag.Id(),
					IsController:    model.IsControllerModel(),
					Cloud:           model.CloudName(),
					CloudRegion:     model.CloudRegion(),
					CloudCredential: credential.Id(),
					SLA:             multiwatcher.ModelSLAInfo{Level: "unsupported"},
					UserPermissions: map[string]permission.Access{
						"bob":  permission.ReadAccess,
						"mary": permission.AdminAccess,
					},
				}}}
		},

		func(c *gc.C, st *State) changeTestCase {
			model, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)

			_, err = model.AddUser(UserAccessSpec{
				User:      names.NewUserTag("tony@external"),
				CreatedBy: model.Owner(),
				Access:    permission.WriteAccess,
			})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "adding a model user updates model",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID: st.ModelUUID(),
					Name:      model.Name(),
					// Existence doesn't care about the other values, and they are
					// not entirely relevant to this test.
					UserPermissions: map[string]permission.Access{
						"bob":  permission.ReadAccess,
						"mary": permission.AdminAccess,
					},
				}},
				change: watcher.Change{
					C:  permissionsC,
					Id: permissionID(modelKey(st.ModelUUID()), userGlobalKey("tony@external")),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID: st.ModelUUID(),
					Name:      model.Name(),
					// When the permissions are updated, only the user permissions are changed.
					UserPermissions: map[string]permission.Access{
						"bob":           permission.ReadAccess,
						"mary":          permission.AdminAccess,
						"tony@external": permission.WriteAccess,
					},
				}}}
		},

		func(c *gc.C, st *State) changeTestCase {
			model, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "removing a permission document removes user permission",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID: st.ModelUUID(),
					Name:      model.Name(),
					// Existence doesn't care about the other values, and they are
					// not entirely relevant to this test.
					UserPermissions: map[string]permission.Access{
						"bob":  permission.ReadAccess,
						"mary": permission.AdminAccess,
					},
				}},
				change: watcher.Change{
					C:     permissionsC,
					Id:    permissionID(modelKey(st.ModelUUID()), userGlobalKey("bob")),
					Revno: -1,
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID: st.ModelUUID(),
					Name:      model.Name(),
					// When the permissions are updated, only the user permissions are changed.
					UserPermissions: map[string]permission.Access{
						"mary": permission.AdminAccess,
					},
				}}}
		},

		func(c *gc.C, st *State) changeTestCase {
			model, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)

			// With the allModelWatcher variant, this function is called twice
			// within the same test loop, so we look for bob, and if not found,
			// add him in.
			bob, err := st.User(names.NewUserTag("bob"))
			if errors.IsNotFound(err) {
				bob, err = st.AddUser("bob", "", "pwd", "admin")
			}
			c.Assert(err, jc.ErrorIsNil)

			_, err = model.AddUser(UserAccessSpec{
				User:      bob.UserTag(),
				CreatedBy: model.Owner(),
				Access:    permission.WriteAccess,
			})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "updating a permission document updates user permission",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID: st.ModelUUID(),
					Name:      model.Name(),
					// Existence doesn't care about the other values, and they are
					// not entirely relevant to this test.
					UserPermissions: map[string]permission.Access{
						"bob":  permission.ReadAccess,
						"mary": permission.AdminAccess,
					},
				}},
				change: watcher.Change{
					C:  permissionsC,
					Id: permissionID(modelKey(st.ModelUUID()), userGlobalKey("bob")),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ModelInfo{
					ModelUUID: st.ModelUUID(),
					Name:      model.Name(),
					UserPermissions: map[string]permission.Access{
						// Bob's permission updated to write.
						"bob":  permission.WriteAccess,
						"mary": permission.AdminAccess,
					},
				}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeAnnotations(c *gc.C, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no annotation in state, no annotation in store -> do nothing",
				change: watcher.Change{
					C:  "annotations",
					Id: st.docID("m#0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "annotation is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.AnnotationInfo{
					ModelUUID: st.ModelUUID(),
					Tag:       "machine-0",
				}},
				change: watcher.Change{
					C:  "annotations",
					Id: st.docID("m#0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			model, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			err = model.SetAnnotations(m, map[string]string{"foo": "bar", "arble": "baz"})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "annotation is added if it's in backing but not in Store",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
					},
				},
				change: watcher.Change{
					C:  "annotations",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID:   st.ModelUUID(),
						ID:          "0",
						Annotations: map[string]string{"foo": "bar", "arble": "baz"},
					},
					&multiwatcher.AnnotationInfo{
						ModelUUID:   st.ModelUUID(),
						Tag:         "machine-0",
						Annotations: map[string]string{"foo": "bar", "arble": "baz"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			model, err := st.Model()
			c.Assert(err, jc.ErrorIsNil)
			err = model.SetAnnotations(m, map[string]string{
				"arble":  "khroomph",
				"pretty": "",
				"new":    "attr",
			})
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "annotation is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
						Annotations: map[string]string{
							"arble":  "baz",
							"foo":    "bar",
							"pretty": "polly",
						}},
					&multiwatcher.AnnotationInfo{
						ModelUUID: st.ModelUUID(),
						Tag:       "machine-0",
						Annotations: map[string]string{
							"arble":  "baz",
							"foo":    "bar",
							"pretty": "polly",
						},
					}},
				change: watcher.Change{
					C:  "annotations",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
						Annotations: map[string]string{
							"arble": "khroomph",
							"new":   "attr",
						}},
					&multiwatcher.AnnotationInfo{
						ModelUUID: st.ModelUUID(),
						Tag:       "machine-0",
						Annotations: map[string]string{
							"arble": "khroomph",
							"new":   "attr",
						}}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeMachines(c *gc.C, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no machine in state -> do nothing",
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("m#0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no machine in state, no machine in store -> do nothing",
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "machine is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					ModelUUID: st.ModelUUID(),
					ID:        "1",
				}},
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Error,
				Message: "failure",
				Since:   &now,
			}
			err = m.SetStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "machine is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
						AgentStatus: multiwatcher.StatusInfo{
							Current: status.Error,
							Message: "failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						InstanceStatus: multiwatcher.StatusInfo{
							Current: status.Pending,
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						Life:      life.Alive,
						Base:      "ubuntu@12.10",
						Jobs:      []coremodel.MachineJob{JobHostUnits.ToParams()},
						Addresses: []network.ProviderAddress{},
						HasVote:   false,
						WantsVote: false,
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine(UbuntuBase("22.04"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetProvisioned("i-0", "", "bootstrap_nonce", nil)
			c.Assert(err, jc.ErrorIsNil)
			err = m.SetSupportedContainers([]instance.ContainerType{instance.LXD})
			c.Assert(err, jc.ErrorIsNil)

			err = m.SetAgentVersion(version.MustParseBinary("2.4.1-ubuntu-amd64"))
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			return changeTestCase{
				about: "machine is updated if it's in backing and in Store",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
						AgentStatus: multiwatcher.StatusInfo{
							Current: status.Error,
							Message: "another failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						InstanceStatus: multiwatcher.StatusInfo{
							Current: status.Pending,
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					},
				},
				change: watcher.Change{
					C:  "machines",
					Id: st.docID("0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID:  st.ModelUUID(),
						ID:         "0",
						InstanceID: "i-0",
						AgentStatus: multiwatcher.StatusInfo{
							Current: status.Error,
							Message: "another failure",
							Data:    map[string]interface{}{},
							Version: "2.4.1",
							Since:   &now,
						},
						InstanceStatus: multiwatcher.StatusInfo{
							Current: status.Pending,
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						Life:                     life.Alive,
						Base:                     "ubuntu@22.04",
						Jobs:                     []coremodel.MachineJob{JobHostUnits.ToParams()},
						Addresses:                []network.ProviderAddress{},
						HardwareCharacteristics:  &instance.HardwareCharacteristics{},
						CharmProfiles:            []string{},
						SupportedContainers:      []instance.ContainerType{instance.LXD},
						SupportedContainersKnown: true,
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			now := st.clock().Now()
			return changeTestCase{
				about: "no change if status is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					ModelUUID: st.ModelUUID(),
					ID:        "0",
					AgentStatus: multiwatcher.StatusInfo{
						Current: status.Error,
						Message: "failure",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
						AgentStatus: multiwatcher.StatusInfo{
							Current: status.Error,
							Message: "failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Started,
				Message: "",
				Since:   &now,
			}
			err = m.SetStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the machine exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					ModelUUID: st.ModelUUID(),
					ID:        "0",
					AgentStatus: multiwatcher.StatusInfo{
						Current: status.Error,
						Message: "failure",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("m#0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
						AgentStatus: multiwatcher.StatusInfo{
							Current: status.Started,
							Data:    make(map[string]interface{}),
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if instanceData is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					ModelUUID: st.ModelUUID(),
					ID:        "0",
				}},
				change: watcher.Change{
					C:  "instanceData",
					Id: st.docID("0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)

			hc := &instance.HardwareCharacteristics{}
			err = m.SetProvisioned(instance.Id("i-"+m.Tag().String()), "", "fake_nonce", hc)
			c.Assert(err, jc.ErrorIsNil)

			profiles := []string{"default, juju-default"}
			err = m.SetCharmProfiles(profiles)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "instanceData is changed (CharmProfiles) if the machine exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.MachineInfo{
					ModelUUID: st.ModelUUID(),
					ID:        "0",
				}},
				change: watcher.Change{
					C:  "instanceData",
					Id: st.docID("0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID:               st.ModelUUID(),
						ID:                      "0",
						CharmProfiles:           profiles,
						HardwareCharacteristics: hc,
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeRelations(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no relation in state, no application in store -> do nothing",
				change: watcher.Change{
					C:  "relations",
					Id: st.docID("logging:logging-directory wordpress:logging-dir"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "relation is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.RelationInfo{
					ModelUUID: st.ModelUUID(),
					Key:       "logging:logging-directory wordpress:logging-dir",
				}},
				change: watcher.Change{
					C:  "relations",
					Id: st.docID("logging:logging-directory wordpress:logging-dir"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			AddTestingApplication(c, st, "logging", AddTestingCharm(c, st, "logging"))
			eps, err := st.InferEndpoints("logging", "wordpress")
			c.Assert(err, jc.ErrorIsNil)
			_, err = st.AddRelation(eps...)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "relation is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "relations",
					Id: st.docID("logging:logging-directory wordpress:logging-dir"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.RelationInfo{
						ModelUUID: st.ModelUUID(),
						Key:       "logging:logging-directory wordpress:logging-dir",
						Endpoints: []multiwatcher.Endpoint{
							{ApplicationName: "logging", Relation: multiwatcher.CharmRelation{Name: "logging-directory", Role: "requirer", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}},
							{ApplicationName: "wordpress", Relation: multiwatcher.CharmRelation{Name: "logging-dir", Role: "provider", Interface: "logging", Optional: false, Limit: 0, Scope: "container"}}},
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeApplications(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	// TODO(wallyworld) - add test for changing application status when that is implemented
	changeTestFuncs := []changeTestFunc{
		// Applications.
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no application in state, no application in store -> do nothing",
				change: watcher.Change{
					C:  "applications",
					Id: st.docID("wordpress"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "application is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "wordpress",
					},
				},
				change: watcher.Change{
					C:  "applications",
					Id: st.docID("wordpress"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			err := wordpress.MergeExposeSettings(nil)
			c.Assert(err, jc.ErrorIsNil)
			err = wordpress.SetMinUnits(42)
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			return changeTestCase{
				about: "application is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "applications",
					Id: st.docID("wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress",
						Exposed:     true,
						CharmURL:    "local:quantal/quantal-wordpress-3",
						Life:        life.Alive,
						MinUnits:    42,
						Config:      charm.Settings{},
						Constraints: constraints.MustParse("arch=amd64"),
						Status: multiwatcher.StatusInfo{
							Current: "unset",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			setApplicationConfigAttr(c, app, "blog-title", "boring")

			return changeTestCase{
				about: "application is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress",
					Exposed:     true,
					CharmURL:    "local:quantal/quantal-wordpress-3",
					MinUnits:    47,
					Constraints: constraints.MustParse("mem=99M"),
					Config:      charm.Settings{"blog-title": "boring"},
				}},
				change: watcher.Change{
					C:  "applications",
					Id: st.docID("wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress",
						CharmURL:    "local:quantal/quantal-wordpress-3",
						Life:        life.Alive,
						Constraints: constraints.MustParse("mem=99M"),
						Config:      charm.Settings{"blog-title": "boring"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			unit, err := app.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			err = unit.SetWorkloadVersion("42.47")
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "workload version is updated when set on a unit",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
					},
					&multiwatcher.ApplicationInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "wordpress",
						CharmURL:  "local:quantal/quantal-wordpress-3",
					},
				},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#" + unit.Name() + "#charm#sat#workload-version"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID:       st.ModelUUID(),
						Name:            "wordpress",
						CharmURL:        "local:quantal/quantal-wordpress-3",
						WorkloadVersion: "42.47",
					},
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
					},
				},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			unit, err := app.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			err = unit.SetWorkloadVersion("")
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "workload version is not updated when empty",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
					},
					&multiwatcher.ApplicationInfo{
						ModelUUID:       st.ModelUUID(),
						Name:            "wordpress",
						CharmURL:        "local:quantal/quantal-wordpress-3",
						WorkloadVersion: "ultimate",
					},
				},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#" + unit.Name() + "#charm#sat#workload-version"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID:       st.ModelUUID(),
						Name:            "wordpress",
						CharmURL:        "local:quantal/quantal-wordpress-3",
						WorkloadVersion: "ultimate",
					},
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
					},
				},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			unit, err := app.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			err = unit.SetWorkloadVersion("42.47")
			c.Assert(err, jc.ErrorIsNil)
			return changeTestCase{
				about: "workload version is ignored if there is no application info",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
					},
				},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#" + unit.Name() + "#charm#sat#workload-version"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
					},
				},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			setApplicationConfigAttr(c, app, "blog-title", "boring")

			return changeTestCase{
				about: "application re-reads config when charm URL changes",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID: st.ModelUUID(),
					Name:      "wordpress",
					// Note: CharmURL has a different revision number from
					// the wordpress revision in the testing repo.
					CharmURL: "local:quantal/quantal-wordpress-2",
					Config:   charm.Settings{"foo": "bar"},
				}},
				change: watcher.Change{
					C:  "applications",
					Id: st.docID("wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "wordpress",
						CharmURL:  "local:quantal/quantal-wordpress-3",
						Life:      life.Alive,
						Config:    charm.Settings{"blog-title": "boring"},
					}}}
		},
		// Settings.
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no application in state -> do nothing",
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("a#dummy-application#local:quantal/quantal-dummy-1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if application is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID: st.ModelUUID(),
					Name:      "dummy-application",
					CharmURL:  "local:quantal/quantal-dummy-1",
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("a#dummy-application#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID: st.ModelUUID(),
					Name:      "dummy-application",
					CharmURL:  "local:quantal/quantal-dummy-1",
				}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "dummy-application", AddTestingCharm(c, st, "dummy"))
			setApplicationConfigAttr(c, app, "username", "foo")
			setApplicationConfigAttr(c, app, "outlook", "foo@bar")

			return changeTestCase{
				about: "application config is changed if application exists in the store with the same URL",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID: st.ModelUUID(),
					Name:      "dummy-application",
					CharmURL:  "local:quantal/quantal-dummy-1",
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("a#dummy-application#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "dummy-application",
						CharmURL:  "local:quantal/quantal-dummy-1",
						Config:    charm.Settings{"username": "foo", "outlook": "foo@bar"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "dummy-application", AddTestingCharm(c, st, "dummy"))
			setApplicationConfigAttr(c, app, "username", "foo")
			setApplicationConfigAttr(c, app, "outlook", "foo@bar")
			setApplicationConfigAttr(c, app, "username", nil)

			return changeTestCase{
				about: "application config is changed after removing of a setting",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID: st.ModelUUID(),
					Name:      "dummy-application",
					CharmURL:  "local:quantal/quantal-dummy-1",
					Config:    charm.Settings{"username": "foo", "outlook": "foo@bar"},
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("a#dummy-application#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "dummy-application",
						CharmURL:  "local:quantal/quantal-dummy-1",
						Config:    charm.Settings{"outlook": "foo@bar"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			testCharm := AddCustomCharm(
				c, st, "dummy",
				"config.yaml", dottedConfig,
				"quantal", 1)
			app := AddTestingApplication(c, st, "dummy-application", testCharm)
			setApplicationConfigAttr(c, app, "key.dotted", "foo")

			return changeTestCase{
				about: "application config is unescaped when reading from the backing store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID: st.ModelUUID(),
					Name:      "dummy-application",
					CharmURL:  "local:quantal/quantal-dummy-1",
					Config:    charm.Settings{"key.dotted": "bar"},
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("a#dummy-application#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "dummy-application",
						CharmURL:  "local:quantal/quantal-dummy-1",
						Config:    charm.Settings{"key.dotted": "foo"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "dummy-application", AddTestingCharm(c, st, "dummy"))
			setApplicationConfigAttr(c, app, "username", "foo")

			return changeTestCase{
				about: "application config is unchanged if application exists in the store with a different URL",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID: st.ModelUUID(),
					Name:      "dummy-application",
					CharmURL:  "local:quantal/quantal-dummy-2", // Note different revno.
					Config:    charm.Settings{"username": "bar"},
				}},
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("a#dummy-application#local:quantal/quantal-dummy-1"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "dummy-application",
						CharmURL:  "local:quantal/quantal-dummy-2",
						Config:    charm.Settings{"username": "bar"},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "non-application config change is ignored",
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("m#0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "application config change with no charm url is ignored",
				change: watcher.Change{
					C:  "settings",
					Id: st.docID("a#foo"),
				}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeCharms(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no charm in state, no charm in store -> do nothing",
				change: watcher.Change{
					C:  "charms",
					Id: st.docID("wordpress"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "charm is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.CharmInfo{
						ModelUUID: st.ModelUUID(),
						CharmURL:  "local:quantal/quantal-wordpress-2",
					},
				},
				change: watcher.Change{
					C:  "charms",
					Id: st.docID("local:quantal/quantal-wordpress-2"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			ch := AddTestingCharm(c, st, "wordpress")
			return changeTestCase{
				about: "charm is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "charms",
					Id: st.docID(ch.URL()),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.CharmInfo{
						ModelUUID:     st.ModelUUID(),
						CharmURL:      ch.URL(),
						Life:          life.Alive,
						DefaultConfig: map[string]interface{}{"blog-title": "My Title"},
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeApplicationsConstraints(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no application in state -> do nothing",
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("a#wordpress"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if application is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M"),
				}},
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("a#wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M"),
				}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			app := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			err := app.SetConstraints(constraints.MustParse("mem=4G arch=amd64"))
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the application exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.ApplicationInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress",
					Constraints: constraints.MustParse("mem=99M cores=2 cpu-power=4"),
				}},
				change: watcher.Change{
					C:  "constraints",
					Id: st.docID("a#wordpress"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress",
						Constraints: constraints.MustParse("mem=4G arch=amd64"),
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeUnits(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no unit in state, no unit in store -> do nothing",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "unit is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "wordpress/1",
						Life:      life.Alive,
					},
				},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/1"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			MustOpenUnitPortRanges(c, u.st, m, u.Name(), allEndpoints, []corenetwork.PortRange{
				corenetwork.MustParsePortRange("12345/tcp"),
				corenetwork.MustParsePortRange("54321/udp"),
				corenetwork.MustParsePortRange("5555-5558/tcp"),
			})
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Error,
				Message: "failure",
				Since:   &now,
			}
			err = u.SetAgentStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						Base:        "ubuntu@12.10",
						Life:        life.Alive,
						MachineID:   "0",
						OpenPortRangesByEndpoint: network.GroupedPortRanges{
							allEndpoints: {
								corenetwork.MustParsePortRange("5555-5558/tcp"),
								corenetwork.MustParsePortRange("12345/tcp"),
								corenetwork.MustParsePortRange("54321/udp"),
							},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			MustOpenUnitPortRange(c, st, m, u.Name(), allEndpoints, corenetwork.MustParsePortRange("17070/udp"))
			err = u.SetAgentVersion(version.MustParseBinary("2.4.1-ubuntu-amd64"))
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()

			return changeTestCase{
				about: "unit is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					ModelUUID: st.ModelUUID(),
					Name:      "wordpress/0",
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "error",
						Message: "another failure",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					OpenPortRangesByEndpoint: network.GroupedPortRanges{
						allEndpoints: {
							corenetwork.MustParsePortRange("17070/udp"),
						},
					},
				}},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						Base:        "ubuntu@12.10",
						Life:        life.Alive,
						MachineID:   "0",
						OpenPortRangesByEndpoint: network.GroupedPortRanges{
							allEndpoints: {
								corenetwork.MustParsePortRange("17070/udp"),
							},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Version: "2.4.1",
							Since:   &now,
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "another failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			MustOpenUnitPortRange(c, st, m, u.Name(), allEndpoints, corenetwork.MustParsePortRange("4242/tcp"))

			return changeTestCase{
				about: "unit info is updated if a port is opened on the machine it is placed in",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "wordpress/0",
					},
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
					},
				},
				change: watcher.Change{
					C:  openedPortsC,
					Id: st.docID("0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID: st.ModelUUID(),
						Name:      "wordpress/0",
						OpenPortRangesByEndpoint: network.GroupedPortRanges{
							allEndpoints: {
								corenetwork.MustParsePortRange("4242/tcp"),
							},
						},
					},
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
					},
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			MustOpenUnitPortRange(c, st, m, u.Name(), allEndpoints, corenetwork.MustParsePortRange("21-22/tcp"))
			now := st.clock().Now()
			return changeTestCase{
				about: "unit is created if a port is opened on the machine it is placed in",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
					},
				},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						Base:        "ubuntu@12.10",
						Life:        life.Alive,
						MachineID:   "0",
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "waiting",
							Message: "waiting for machine",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						OpenPortRangesByEndpoint: network.GroupedPortRanges{
							allEndpoints: {
								corenetwork.MustParsePortRange("21-22/tcp"),
							},
						},
					},
					&multiwatcher.MachineInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "0",
					},
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
			MustOpenUnitPortRange(c, st, m, u.Name(), allEndpoints, corenetwork.MustParsePortRange("12345/tcp"))
			publicAddress := network.NewSpaceAddress("public", corenetwork.WithScope(corenetwork.ScopePublic))
			privateAddress := network.NewSpaceAddress("private", corenetwork.WithScope(corenetwork.ScopeCloudLocal))
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Error,
				Message: "failure",
				Since:   &now,
			}
			err = u.SetAgentStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit addresses are read from the assigned machine for recent Juju releases",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.MachineInfo{
						ModelUUID:               st.ModelUUID(),
						ID:                      "0",
						PreferredPublicAddress:  publicAddress,
						PreferredPrivateAddress: privateAddress,
					},
				},
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:      st.ModelUUID(),
						Name:           "wordpress/0",
						Application:    "wordpress",
						Base:           "ubuntu@12.10",
						Life:           life.Alive,
						PublicAddress:  "public",
						PrivateAddress: "private",
						MachineID:      "0",
						OpenPortRangesByEndpoint: network.GroupedPortRanges{
							allEndpoints: {
								corenetwork.MustParsePortRange("12345/tcp"),
							},
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					},
					&multiwatcher.MachineInfo{
						ModelUUID:               st.ModelUUID(),
						ID:                      "0",
						PreferredPublicAddress:  publicAddress,
						PreferredPrivateAddress: privateAddress,
					},
				},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no unit in state -> do nothing",
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			now := st.clock().Now()
			return changeTestCase{
				about: "no change if status is not in backing",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress/0",
					Application: "wordpress",
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "error",
						Message: "failure",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "failure",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			err = u.AssignToNewMachine()
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Idle,
				Message: "",
				Since:   &now,
			}
			err = u.SetAgentStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "status is changed if the unit exists in the store",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress/0",
					Application: "wordpress",
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "maintenance",
						Message: "working",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "maintenance",
							Message: "working",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Idle,
				Message: "",
				Since:   &now,
			}
			err = u.AssignToNewMachine()
			c.Assert(err, jc.ErrorIsNil)
			err = u.SetAgentStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)
			sInfo = status.StatusInfo{
				Status:  status.Maintenance,
				Message: "doing work",
				Since:   &now,
			}
			err = u.SetStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "unit status is changed if the agent comes off error state",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress/0",
					Application: "wordpress",
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "error",
						Message: "failure",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "maintenance",
							Message: "doing work",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Error,
				Message: "hook error",
				Data: map[string]interface{}{
					"1st-key": "one",
					"2nd-key": 2,
					"3rd-key": true,
				},
				Since: &now,
			}
			err = u.SetAgentStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "agent status is changed with additional status data",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress/0",
					Application: "wordpress",
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "active",
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "hook error",
							Data: map[string]interface{}{
								"1st-key": "one",
								"2nd-key": 2,
								"3rd-key": true,
							},
							Since: &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Error,
				Message: "hook error",
				Data: map[string]interface{}{
					"1st-key": "one",
					"2nd-key": 2,
					"3rd-key": true,
				},
				Since: &now,
			}
			err = u.SetAgentStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "workload status takes into account agent error",
				initialContents: []multiwatcher.EntityInfo{&multiwatcher.UnitInfo{
					ModelUUID:   st.ModelUUID(),
					Name:        "wordpress/0",
					Application: "wordpress",
					WorkloadStatus: multiwatcher.StatusInfo{
						Current: "error",
						Message: "hook error",
						Data: map[string]interface{}{
							"1st-key": "one",
							"2nd-key": 2,
							"3rd-key": true,
						},
						Since: &now,
					},
					AgentStatus: multiwatcher.StatusInfo{
						Current: "idle",
						Message: "",
						Data:    map[string]interface{}{},
						Since:   &now,
					},
				}},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID("u#wordpress/0#charm"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "error",
							Message: "hook error",
							Data: map[string]interface{}{
								"1st-key": "one",
								"2nd-key": 2,
								"3rd-key": true,
							},
							Since: &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "idle",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

// initFlag helps to control the different test scenarios.
type initFlag int

const (
	assignUnit initFlag = 1
	openPorts  initFlag = 2
	closePorts initFlag = 4
)

func testChangeUnitsNonNilPorts(c *gc.C, owner names.UserTag, runChangeTests func(*gc.C, []changeTestFunc)) {
	initModel := func(c *gc.C, st *State, flag initFlag) {
		wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
		u, err := wordpress.AddUnit(AddUnitParams{})
		c.Assert(err, jc.ErrorIsNil)
		m, err := st.AddMachine(UbuntuBase("12.10"), JobHostUnits)
		c.Assert(err, jc.ErrorIsNil)
		if flag&assignUnit != 0 {
			// Assign the unit.
			err = u.AssignToMachine(m)
			c.Assert(err, jc.ErrorIsNil)
		}
		if flag&openPorts != 0 {
			// Add a network to the machine and open a port.
			publicAddress := network.NewSpaceAddress("1.2.3.4", corenetwork.WithScope(corenetwork.ScopePublic))
			privateAddress := network.NewSpaceAddress("4.3.2.1", corenetwork.WithScope(corenetwork.ScopeCloudLocal))
			err = m.SetProviderAddresses(publicAddress, privateAddress)
			c.Assert(err, jc.ErrorIsNil)

			unitPortRanges, err := u.OpenedPortRanges()
			if flag&assignUnit == 0 {
				c.Assert(err, jc.Satisfies, errors.IsNotAssigned)
			} else {
				c.Assert(err, jc.ErrorIsNil)
				unitPortRanges.Open(allEndpoints, corenetwork.MustParsePortRange("12345/tcp"))
				c.Assert(st.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
			}
		}
		if flag&closePorts != 0 {
			// Close the port again (only if been opened before).
			unitPortRanges, err := u.OpenedPortRanges()
			c.Assert(err, jc.ErrorIsNil)
			unitPortRanges.Close(allEndpoints, corenetwork.MustParsePortRange("12345/tcp"))
			c.Assert(st.ApplyOperation(unitPortRanges.Changes()), jc.ErrorIsNil)
		}
	}
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			initModel(c, st, assignUnit)
			now := st.clock().Now()
			return changeTestCase{
				about: "don't open ports on unit",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						Base:        "ubuntu@12.10",
						Life:        life.Alive,
						MachineID:   "0",
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "waiting",
							Message: "waiting for machine",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			initModel(c, st, assignUnit|openPorts)
			now := st.clock().Now()

			return changeTestCase{
				about: "open a port on unit",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						Base:        "ubuntu@12.10",
						Life:        life.Alive,
						MachineID:   "0",
						OpenPortRangesByEndpoint: network.GroupedPortRanges{
							allEndpoints: {corenetwork.MustParsePortRange("12345/tcp")},
						},
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "waiting",
							Message: "waiting for machine",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			initModel(c, st, assignUnit|openPorts|closePorts)
			now := st.clock().Now()

			return changeTestCase{
				about: "open a port on unit and close it again",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						Base:        "ubuntu@12.10",
						Life:        life.Alive,
						MachineID:   "0",
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "waiting",
							Message: "waiting for machine",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
		func(c *gc.C, st *State) changeTestCase {
			initModel(c, st, openPorts)
			now := st.clock().Now()

			return changeTestCase{
				about: "open ports on an unassigned unit",
				change: watcher.Change{
					C:  "units",
					Id: st.docID("wordpress/0"),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.UnitInfo{
						ModelUUID:   st.ModelUUID(),
						Name:        "wordpress/0",
						Application: "wordpress",
						Base:        "ubuntu@12.10",
						Life:        life.Alive,
						WorkloadStatus: multiwatcher.StatusInfo{
							Current: "waiting",
							Message: "waiting for machine",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
						AgentStatus: multiwatcher.StatusInfo{
							Current: "allocating",
							Message: "",
							Data:    map[string]interface{}{},
							Since:   &now,
						},
					}}}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeRemoteApplications(c *gc.C, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no remote application in state, no remote application in store -> do nothing",
				change: watcher.Change{
					C:  "remoteApplications",
					Id: st.docID("remote-mysql2"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "remote application is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.RemoteApplicationUpdate{
						ModelUUID: st.ModelUUID(),
						Name:      "remote-mysql2",
						OfferURL:  "me/model.mysql",
					},
				},
				change: watcher.Change{
					C:  "remoteApplications",
					Id: st.docID("remote-mysql2"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			_, remoteApplicationInfo := addTestingRemoteApplication(
				c, st, "remote-mysql2", "me/model.mysql", mysqlRelations, false)
			return changeTestCase{
				about: "remote application is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "remoteApplications",
					Id: st.docID("remote-mysql2"),
				},
				expectContents: []multiwatcher.EntityInfo{&remoteApplicationInfo},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			// Currently the only change we can make to a remote
			// application is to destroy it.
			//
			// We must add a relation to the remote application, and
			// a unit to the relation, so that the relation is not
			// removed and thus the remote application is not removed
			// upon destroying.
			wordpress := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			mysql, remoteApplicationInfo := addTestingRemoteApplication(
				c, st, "remote-mysql2", "me/model.mysql", mysqlRelations, false,
			)

			eps, err := st.InferEndpoints("wordpress", "remote-mysql2")
			c.Assert(err, jc.ErrorIsNil)
			rel, err := st.AddRelation(eps[0], eps[1])
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(wordpress.Refresh(), jc.ErrorIsNil)
			c.Assert(mysql.Refresh(), jc.ErrorIsNil)

			wu, err := wordpress.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)
			wru, err := rel.Unit(wu)
			c.Assert(err, jc.ErrorIsNil)
			err = wru.EnterScope(nil)
			c.Assert(err, jc.ErrorIsNil)

			status, err := mysql.Status()
			c.Assert(err, jc.ErrorIsNil)

			err = mysql.Destroy()
			c.Assert(err, jc.ErrorIsNil)

			now := st.clock().Now()
			initialRemoteApplicationInfo := remoteApplicationInfo
			remoteApplicationInfo.Life = "dying"
			remoteApplicationInfo.Status = multiwatcher.StatusInfo{
				Current: status.Status,
				Message: status.Message,
				Data:    status.Data,
				Since:   &now,
			}
			return changeTestCase{
				about:           "remote application is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&initialRemoteApplicationInfo},
				change: watcher.Change{
					C:  "remoteApplications",
					Id: st.docID("remote-mysql2"),
				},
				expectContents: []multiwatcher.EntityInfo{&remoteApplicationInfo},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			mysql, remoteApplicationInfo := addTestingRemoteApplication(
				c, st, "remote-mysql2", "me/model.mysql", mysqlRelations, false,
			)
			now := st.clock().Now()
			sInfo := status.StatusInfo{
				Status:  status.Active,
				Message: "running",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			}
			err := mysql.SetStatus(sInfo)
			c.Assert(err, jc.ErrorIsNil)
			initialRemoteApplicationInfo := remoteApplicationInfo
			remoteApplicationInfo.Status = multiwatcher.StatusInfo{
				Current: "active",
				Message: "running",
				Data:    map[string]interface{}{"foo": "bar"},
				Since:   &now,
			}
			return changeTestCase{
				about:           "remote application status is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&initialRemoteApplicationInfo},
				change: watcher.Change{
					C:  "statuses",
					Id: st.docID(mysql.globalKey()),
				},
				expectContents: []multiwatcher.EntityInfo{&remoteApplicationInfo},
			}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeApplicationOffers(c *gc.C, runChangeTests func(*gc.C, []changeTestFunc)) {
	addOffer := func(c *gc.C, st *State) (multiwatcher.ApplicationOfferInfo, *User) {
		owner, err := st.AddUser("owner", "owner", "password", "admin")
		c.Assert(err, jc.ErrorIsNil)
		AddTestingApplication(c, st, "mysql", AddTestingCharm(c, st, "mysql"))
		addTestingRemoteApplication(
			c, st, "remote-wordpress", "", []charm.Relation{{
				Name:      "db",
				Role:      "requirer",
				Scope:     charm.ScopeGlobal,
				Interface: "mysql",
			}}, true,
		)
		applicationOfferInfo, _ := addTestingApplicationOffer(
			c, st, owner.UserTag(), "hosted-mysql", "mysql",
			"quantal-mysql", []string{"server"})
		return applicationOfferInfo, owner
	}

	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no application offer in state, no application offer in store -> do nothing",
				change: watcher.Change{
					C:  "applicationOffers",
					Id: st.docID("hosted-mysql"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "application offer is removed if it's not in backing",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.ApplicationOfferInfo{
						ModelUUID:       st.ModelUUID(),
						OfferName:       "hosted-mysql",
						OfferUUID:       "hosted-mysql-uuid",
						ApplicationName: "mysql",
					},
				},
				change: watcher.Change{
					C:  "applicationOffers",
					Id: st.docID("hosted-mysql"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			applicationOfferInfo, _ := addOffer(c, st)
			return changeTestCase{
				about: "application offer is added if it's in backing but not in Store",
				change: watcher.Change{
					C:  "applicationOffers",
					Id: st.docID("hosted-mysql"),
				},
				expectContents: []multiwatcher.EntityInfo{&applicationOfferInfo},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			applicationOfferInfo, owner := addOffer(c, st)
			app, err := st.Application("mysql")
			c.Assert(err, jc.ErrorIsNil)
			curl, _ := app.CharmURL()
			ch, err := st.Charm(*curl)
			c.Assert(err, jc.ErrorIsNil)
			AddTestingApplication(c, st, "another-mysql", ch)
			offers := NewApplicationOffers(st)
			_, err = offers.UpdateOffer(crossmodel.AddApplicationOfferArgs{
				OfferName:       "hosted-mysql",
				Owner:           owner.Name(),
				ApplicationName: "another-mysql",
				Endpoints:       map[string]string{"server": "server"},
			})
			c.Assert(err, jc.ErrorIsNil)

			initialApplicationOfferInfo := applicationOfferInfo
			applicationOfferInfo.ApplicationName = "another-mysql"
			return changeTestCase{
				about:           "application offer is updated if it's in backing and not in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&initialApplicationOfferInfo},
				change: watcher.Change{
					C:  "applicationOffers",
					Id: st.docID("hosted-mysql"),
				},
				expectContents: []multiwatcher.EntityInfo{&applicationOfferInfo},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			applicationOfferInfo, _ := addOffer(c, st)
			initialApplicationOfferInfo := applicationOfferInfo
			addTestingRemoteApplication(
				c, st, "remote-wordpress2", "", []charm.Relation{{
					Name:      "db",
					Role:      "requirer",
					Scope:     charm.ScopeGlobal,
					Interface: "mysql",
				}}, true,
			)
			eps, err := st.InferEndpoints("mysql", "remote-wordpress2")
			c.Assert(err, jc.ErrorIsNil)
			rel, err := st.AddRelation(eps...)
			c.Assert(err, jc.ErrorIsNil)
			_, err = st.AddOfferConnection(AddOfferConnectionParams{
				SourceModelUUID: utils.MustNewUUID().String(),
				RelationId:      rel.Id(),
				RelationKey:     rel.Tag().Id(),
				Username:        "fred",
				OfferUUID:       initialApplicationOfferInfo.OfferUUID,
			})
			c.Assert(err, jc.ErrorIsNil)

			applicationOfferInfo.TotalConnectedCount = 2
			return changeTestCase{
				about:           "application offer count is updated if it's in backing and in multiwatcher.Store",
				initialContents: []multiwatcher.EntityInfo{&initialApplicationOfferInfo},
				change: watcher.Change{
					C:  "remoteApplications",
					Id: st.docID("remote-wordpress2"),
				},
				expectContents: []multiwatcher.EntityInfo{&applicationOfferInfo},
			}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

func testChangeGenerations(c *gc.C, runChangeTests func(*gc.C, []changeTestFunc)) {
	changeTestFuncs := []changeTestFunc{
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "no change if generation absent from state and store",
				change: watcher.Change{
					C:  "generations",
					Id: st.docID("does-not-exist"),
				}}
		},
		func(c *gc.C, st *State) changeTestCase {
			return changeTestCase{
				about: "generation is removed if not in backing",
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.BranchInfo{
						ModelUUID: st.ModelUUID(),
						ID:        "to-be-removed",
					},
				},
				change: watcher.Change{
					C:  "generations",
					Id: st.docID("to-be-removed"),
				},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			c.Assert(st.AddBranch("new-branch", "some-user"), jc.ErrorIsNil)
			branch, err := st.Branch("new-branch")
			c.Assert(err, jc.ErrorIsNil)

			return changeTestCase{
				about: "generation is added if in backing but not in store",
				change: watcher.Change{
					C:  "generations",
					Id: st.docID(branch.doc.DocId),
				},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.BranchInfo{
						ModelUUID:     st.ModelUUID(),
						ID:            st.localID(branch.doc.DocId),
						Name:          "new-branch",
						AssignedUnits: map[string][]string{},
						CreatedBy:     "some-user",
					}},
			}
		},
		func(c *gc.C, st *State) changeTestCase {
			c.Assert(st.AddBranch("new-branch", "some-user"), jc.ErrorIsNil)
			branch, err := st.Branch("new-branch")
			c.Assert(err, jc.ErrorIsNil)

			app := AddTestingApplication(c, st, "wordpress", AddTestingCharm(c, st, "wordpress"))
			u, err := app.AddUnit(AddUnitParams{})
			c.Assert(err, jc.ErrorIsNil)

			c.Assert(branch.AssignUnit(u.Name()), jc.ErrorIsNil)

			return changeTestCase{
				about: "generation is updated if in backing and in store",
				change: watcher.Change{
					C:  "generations",
					Id: st.docID(branch.doc.DocId),
				},
				initialContents: []multiwatcher.EntityInfo{
					&multiwatcher.BranchInfo{
						ModelUUID:     st.ModelUUID(),
						ID:            st.localID(branch.doc.DocId),
						Name:          "new-branch",
						AssignedUnits: map[string][]string{},
						CreatedBy:     "some-user",
					}},
				expectContents: []multiwatcher.EntityInfo{
					&multiwatcher.BranchInfo{
						ModelUUID:     st.ModelUUID(),
						ID:            st.localID(branch.doc.DocId),
						Name:          "new-branch",
						AssignedUnits: map[string][]string{app.Name(): {u.Name()}},
						CreatedBy:     "some-user",
					}},
			}
		},
	}
	runChangeTests(c, changeTestFuncs)
}

type entityInfoSlice []multiwatcher.EntityInfo

func (s entityInfoSlice) Len() int      { return len(s) }
func (s entityInfoSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s entityInfoSlice) Less(i, j int) bool {
	id0, id1 := s[i].EntityID(), s[j].EntityID()
	if id0.Kind != id1.Kind {
		return id0.Kind < id1.Kind
	}
	if id0.ModelUUID != id1.ModelUUID {
		return id0.ModelUUID < id1.ModelUUID
	}
	return id0.ID < id1.ID
}

func makeActionInfo(a Action, st *State) multiwatcher.ActionInfo {
	results, message := a.Results()
	return multiwatcher.ActionInfo{
		ModelUUID:      st.ModelUUID(),
		ID:             a.Id(),
		Receiver:       a.Receiver(),
		Name:           a.Name(),
		Parameters:     a.Parameters(),
		Parallel:       a.Parallel(),
		ExecutionGroup: a.ExecutionGroup(),
		Status:         string(a.Status()),
		Message:        message,
		Results:        results,
		Enqueued:       a.Enqueued(),
		Started:        a.Started(),
		Completed:      a.Completed(),
	}
}

func jcDeepEqualsCheck(c *gc.C, got, want interface{}) bool {
	ok, err := jc.DeepEqual(got, want)
	if ok {
		c.Check(err, jc.ErrorIsNil)
	}
	return ok
}

// assertEntitiesEqual is a specialised version of the typical
// jc.DeepEquals check that provides more informative output when
// comparing EntityInfo slices.
func assertEntitiesEqual(c *gc.C, got, want []multiwatcher.EntityInfo) {
	if jcDeepEqualsCheck(c, got, want) {
		return
	}
	if len(got) != len(want) {
		c.Errorf("entity length mismatch; got %d; want %d", len(got), len(want))
	} else {
		c.Errorf("entity contents mismatch; same length %d", len(got))
	}
	// Lets construct a decent output.
	var errorOutput string
	errorOutput = "\ngot: \n"
	for _, e := range got {
		errorOutput += fmt.Sprintf("  %T %#v\n", e, e)
	}
	errorOutput += "expected: \n"
	for _, e := range want {
		errorOutput += fmt.Sprintf("  %T %#v\n", e, e)
	}

	c.Errorf(errorOutput)

	if len(got) == len(want) {
		for i := 0; i < len(got); i++ {
			g := got[i]
			w := want[i]
			if !jcDeepEqualsCheck(c, g, w) {
				if ok := c.Check(g, jc.DeepEquals, w); !ok {
					c.Logf("first difference at position %d\n", i)
				}
				break
			}
		}
	}
	c.FailNow()
}
