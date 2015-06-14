// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"fmt"
	"time"

	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/lease"
)

type FixtureParams struct {
	Id         string
	Namespace  string
	Collection string
	ClockStart time.Time
	ClockStep  time.Duration
}

type Fixture struct {
	Client lease.Client
	Config lease.ClientConfig
	Runner jujutxn.Runner
	Clock  *Clock
	Zero   time.Time
}

var (
	defaultClient     = "default-client"
	defaultNamespace  = "default-namespace"
	defaultCollection = "default-collection"
	defaultClockStart time.Time
)

func init() {
	// We pick a time with a comfortable h:m:s component but:
	//  (1) way in the future past the uint32 unix epoch limit;
	//  (2) at a 5ns offset to make sure we're not discarding precision;
	//  (3) in a weird time zone.
	value := "2013-03-03T01:00:00.000000005-08:40"
	var err error
	defaultClockStart, err = time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
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

func (fix *Fixture) EarliestExpiry() gc.Checker {
	return &callbackChecker{
		CheckerInfo: &gc.CheckerInfo{
			Name:   fmt.Sprintf("EarliestExpiry[%s]", fix.badge()),
			Params: []string{"name", "expiry"},
		},
		callback: fix.infoChecker(checkEarliestExpiry),
	}
}

func (fix *Fixture) LatestExpiry() gc.Checker {
	return &callbackChecker{
		CheckerInfo: &gc.CheckerInfo{
			Name:   fmt.Sprintf("LatestExpiry[%s]", fix.badge()),
			Params: []string{"name", "expiry"},
		},
		callback: fix.infoChecker(checkLatestExpiry),
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
		info, found := fix.Client.Leases()[name]
		if !found {
			return false, fmt.Sprintf("lease %q not held", name)
		}
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

func checkEarliestExpiry(info lease.Info, expiry interface{}) (bool, string) {
	actual := info.EarliestExpiry
	expect := expiry.(time.Time)
	if actual.Equal(expect) {
		return true, ""
	}
	return false, fmt.Sprintf("earliest expiry is %s; expected %s", actual, expect)
}

func checkLatestExpiry(info lease.Info, expiry interface{}) (bool, string) {
	actual := info.LatestExpiry
	expect := expiry.(time.Time)
	if actual.Equal(expect) {
		return true, ""
	}
	return false, fmt.Sprintf("latest expiry is %s; expected %s", actual, expect)
}

func or(u, v string) string {
	if u != "" {
		return u
	}
	return v
}

// Clock exposes time via Now, and can be controlled via Advance. It can be
// configured to Advance automatically whenever Now is called.
type Clock struct {
	now  time.Time
	step time.Duration
}

func NewClock(now time.Time, step time.Duration) *Clock {
	return &Clock{now, step}
}

// Now is part of the lease.Clock interface.
func (clock *Clock) Now() time.Time {
	defer clock.Advance(clock.step)
	return clock.now
}

func (clock *Clock) Advance(duration time.Duration) {
	clock.now = clock.now.Add(duration)
}

// Mongo exposes database operations. It uses a real database -- we can't mock
// mongo out, we need to check it really actually works -- but it's good to
// have the runner accessible for adversarial transaction tests.
type Mongo struct {
	database *mgo.Database
	runner   jujutxn.Runner
}

func NewMongo(database *mgo.Database) *Mongo {
	return &Mongo{
		database: database,
		runner: jujutxn.NewRunner(jujutxn.RunnerParams{
			Database: database,
		}),
	}
}

// GetCollection is part of the lease.Mongo interface.
func (m *Mongo) GetCollection(name string) (*mgo.Collection, func()) {
	return mongo.CollectionFromName(m.database, name)
}

// RunTransaction is part of the lease.Mongo interface.
func (m *Mongo) RunTransaction(getTxn jujutxn.TransactionSource) error {
	return m.runner.Run(getTxn)
}
