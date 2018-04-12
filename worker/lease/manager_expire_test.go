// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

type ExpireSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ExpireSuite{})

func (s *ExpireSuite) TestStartup_ExpiryInPast(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {Expiry: offset(-time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, _ *testing.Clock) {})
}

func (s *ExpireSuite) TestStartup_ExpiryInFuture(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {Expiry: offset(time.Second)},
		},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testing.Clock) {
		clock.Advance(almostSeconds(1))
	})
}

func (s *ExpireSuite) TestStartup_ExpiryInFuture_TimePasses(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testing.Clock) {
		clock.Advance(time.Second)
	})
}

func (s *ExpireSuite) TestStartup_NoExpiry_NotLongEnough(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(_ *lease.Manager, clock *testing.Clock) {
		clock.Advance(almostSeconds(3600))
	})
}

func (s *ExpireSuite) TestStartup_NoExpiry_LongEnough(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"goose": {Expiry: offset(3 * time.Hour)},
		},
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Expiry: offset(time.Minute),
				}
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testing.Clock) {
		clock.Advance(time.Hour)
	})
}

func (s *ExpireSuite) TestExpire_ErrInvalid_Expired(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			err:    corelease.ErrInvalid,
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testing.Clock) {
		clock.Advance(time.Second)
	})
}

func (s *ExpireSuite) TestExpire_ErrInvalid_Updated(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			err:    corelease.ErrInvalid,
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{Expiry: offset(time.Minute)}
			},
		}},
	}
	fix.RunTest(c, func(_ *lease.Manager, clock *testing.Clock) {
		clock.Advance(time.Second)
	})
}

func (s *ExpireSuite) TestExpire_OtherError(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {Expiry: offset(time.Second)},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			err:    errors.New("snarfblat hobalob"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		clock.Advance(time.Second)
		err := manager.Wait()
		c.Check(err, gc.ErrorMatches, "snarfblat hobalob")
	})
}

func (s *ExpireSuite) TestClaim_ExpiryInFuture(c *gc.C) {
	const newLeaseSecs = 63
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args:   []interface{}{"redis", corelease.Request{"redis/0", time.Minute}},
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(newLeaseSecs * time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		// Ask for a minute, actually get 63s. Don't expire early.
		err := manager.Claim("redis", "redis/0", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		clock.Advance(almostSeconds(newLeaseSecs))
	})
}

func (s *ExpireSuite) TestClaim_ExpiryInFuture_TimePasses(c *gc.C) {
	const newLeaseSecs = 63
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args:   []interface{}{"redis", corelease.Request{"redis/0", time.Minute}},
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(newLeaseSecs * time.Second),
				}
			},
		}, {
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		// Ask for a minute, actually get 63s. Expire on time.
		err := manager.Claim("redis", "redis/0", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		clock.Advance(justAfterSeconds(newLeaseSecs))
	})
}

func (s *ExpireSuite) TestExtend_ExpiryInFuture(c *gc.C) {
	const newLeaseSecs = 63
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args:   []interface{}{"redis", corelease.Request{"redis/0", time.Minute}},
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(newLeaseSecs * time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		// Ask for a minute, actually get 63s. Don't expire early.
		err := manager.Claim("redis", "redis/0", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		clock.Advance(almostSeconds(newLeaseSecs))
	})
}

func (s *ExpireSuite) TestExtend_ExpiryInFuture_TimePasses(c *gc.C) {
	const newLeaseSecs = 63
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args:   []interface{}{"redis", corelease.Request{"redis/0", time.Minute}},
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(newLeaseSecs * time.Second),
				}
			},
		}, {
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		// Ask for a minute, actually get 63s. Expire on time.
		err := manager.Claim("redis", "redis/0", time.Minute)
		c.Assert(err, jc.ErrorIsNil)
		clock.Advance(justAfterSeconds(newLeaseSecs))
	})
}

func (s *ExpireSuite) TestExpire_Multiple(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
			"store": {
				Holder: "store/3",
				Expiry: offset(5 * time.Second),
			},
			"tokumx": {
				Holder: "tokumx/5",
				Expiry: offset(10 * time.Second), // will not expire.
			},
			"ultron": {
				Holder: "ultron/7",
				Expiry: offset(5 * time.Second),
			},
			"vvvvvv": {
				Holder: "vvvvvv/2",
				Expiry: offset(time.Second), // would expire, but errors first.
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{"store"},
			err:    corelease.ErrInvalid,
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "store")
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{"ultron"},
			err:    errors.New("what is this?"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testing.Clock) {
		clock.Advance(5 * time.Second)
		err := manager.Wait()
		c.Check(err, gc.ErrorMatches, "what is this\\?")
	})
}
