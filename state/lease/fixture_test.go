// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"fmt"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/lease"
)

var (
	defaultClient     = "default-client"
	defaultNamespace  = "default-namespace"
	defaultCollection = "default-collection"
	defaultClockStart time.Time
)

func init() {
	// We pick a time with a comfortable h:m:s component but:
	//  (1) past the int32 unix epoch limit;
	//  (2) at a 5ns offset to make sure we're not discarding precision;
	//  (3) in a weird time zone.
	value := "2073-03-03T01:00:00.000000005-08:40"
	var err error
	defaultClockStart, err = time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
}

type FixtureParams struct {
	Id         string
	Namespace  string
	Collection string
	ClockStart time.Time
	ClockStep  time.Duration
}

// Fixture collects together a running client and a bunch of useful data.
type Fixture struct {
	Client lease.Client
	Config lease.ClientConfig
	Runner jujutxn.Runner
	Clock  *Clock
	Zero   time.Time
}

func NewFixture(c *gc.C, database *mgo.Database, params FixtureParams) *Fixture {
	mongo := NewMongo(database)
	clockStart := params.ClockStart
	if clockStart.IsZero() {
		clockStart = defaultClockStart
	}
	clock := NewClock(clockStart, params.ClockStep)
	config := lease.ClientConfig{
		Id:         or(params.Id, "default-client"),
		Namespace:  or(params.Namespace, "default-namespace"),
		Collection: or(params.Collection, "default-collection"),
		Mongo:      mongo,
		Clock:      clock,
	}
	client, err := lease.NewClient(config)
	c.Assert(err, jc.ErrorIsNil)
	return &Fixture{
		Client: client,
		Config: config,
		Runner: mongo.runner,
		Clock:  clock,
		Zero:   clockStart,
	}
}

func or(u, v string) string {
	if u != "" {
		return u
	}
	return v
}

func (fix *Fixture) badge() string {
	return fmt.Sprintf("%s %s", fix.Config.Id, fix.Config.Namespace)
}

func (fix *Fixture) Holder() gc.Checker {
	return &callbackChecker{
		CheckerInfo: &gc.CheckerInfo{
			Name:   fmt.Sprintf("Holder[%s]", fix.badge()),
			Params: []string{"name", "holder"},
		},
		callback: fix.infoChecker(checkHolder),
	}
}

func (fix *Fixture) Expiry() gc.Checker {
	return &callbackChecker{
		CheckerInfo: &gc.CheckerInfo{
			Name:   fmt.Sprintf("Expiry[%s]", fix.badge()),
			Params: []string{"name", "expiry"},
		},
		callback: fix.infoChecker(checkExpiry),
	}
}

func (fix *Fixture) infoChecker(checkInfo checkInfoFunc) checkFunc {

	return func(params []interface{}, names []string) (result bool, error string) {
		defer func() {
			if v := recover(); v != nil {
				result = false
				error = fmt.Sprint(v)
			}
		}()
		name := params[0].(string)
		info := fix.Client.Leases()[name]
		return checkInfo(info, params[1])
	}
}

type callbackChecker struct {
	*gc.CheckerInfo
	callback checkFunc
}

func (c *callbackChecker) Check(params []interface{}, names []string) (bool, string) {
	return c.callback(params, names)
}

type checkFunc func(params []interface{}, names []string) (bool, string)

type checkInfoFunc func(info lease.Info, param interface{}) (bool, string)

func checkHolder(info lease.Info, holder interface{}) (bool, string) {
	actual := info.Holder
	expect := holder.(string)
	if actual == expect {
		return true, ""
	}
	return false, fmt.Sprintf("lease held by %q; expected %q", actual, expect)
}

func checkExpiry(info lease.Info, expiry interface{}) (bool, string) {
	actual := info.Expiry
	expect := expiry.(time.Time)
	if actual.Equal(expect) {
		return true, ""
	}
	return false, fmt.Sprintf("expiry is %s; expected %s", actual, expect)
}

type FixtureSuite struct {
	jujutesting.IsolationSuite
	jujutesting.MgoSuite
	db *mgo.Database
}

func (s *FixtureSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.MgoSuite.SetUpSuite(c)
}

func (s *FixtureSuite) TearDownSuite(c *gc.C) {
	s.MgoSuite.TearDownSuite(c)
	s.IsolationSuite.TearDownSuite(c)
}

func (s *FixtureSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.MgoSuite.SetUpTest(c)
	s.db = s.Session.DB("juju")
}

func (s *FixtureSuite) TearDownTest(c *gc.C) {
	s.MgoSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *FixtureSuite) NewFixture(c *gc.C, fp FixtureParams) *Fixture {
	return NewFixture(c, s.db, fp)
}

func (s *FixtureSuite) EasyFixture(c *gc.C) *Fixture {
	return s.NewFixture(c, FixtureParams{})
}
