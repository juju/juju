// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/testing"
)

type DestroySuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&DestroySuite{})

func (s *DestroySuite) TestCannotGetInstances(c *gc.C) {
	env := &mockEnviron{
		allInstances: func() ([]instance.Instance, error) {
			return nil, fmt.Errorf("nope")
		},
		config: configGetter(c),
	}
	err := common.Destroy(env)
	c.Assert(err, gc.ErrorMatches, "nope")
}

func (s *DestroySuite) TestCannotStopInstances(c *gc.C) {
	env := &mockEnviron{
		allInstances: func() ([]instance.Instance, error) {
			return []instance.Instance{
				&mockInstance{id: "one"},
				&mockInstance{id: "another"},
			}, nil
		},
		stopInstances: func(ids []instance.Id) error {
			c.Assert(ids, gc.HasLen, 2)
			c.Assert(ids[0], gc.Equals, instance.Id("one"))
			c.Assert(ids[1], gc.Equals, instance.Id("another"))
			return fmt.Errorf("nah")
		},
		config: configGetter(c),
	}
	err := common.Destroy(env)
	c.Assert(err, gc.ErrorMatches, "nah")
}

func (s *DestroySuite) TestCannotTrashStorage(c *gc.C) {
	env := &mockEnviron{
		storage: &mockStorage{removeAllErr: fmt.Errorf("noes!")},
		allInstances: func() ([]instance.Instance, error) {
			return []instance.Instance{
				&mockInstance{id: "one"},
				&mockInstance{id: "another"},
			}, nil
		},
		stopInstances: func(ids []instance.Id) error {
			c.Assert(ids, gc.HasLen, 2)
			c.Assert(ids[0], gc.Equals, instance.Id("one"))
			c.Assert(ids[1], gc.Equals, instance.Id("another"))
			return nil
		},
		config: configGetter(c),
	}
	err := common.Destroy(env)
	c.Assert(err, gc.ErrorMatches, "noes!")
}

func (s *DestroySuite) TestSuccess(c *gc.C) {
	stor := newStorage(s, c)
	err := stor.Put("somewhere", strings.NewReader("stuff"), 5)
	c.Assert(err, gc.IsNil)

	env := &mockEnviron{
		storage: stor,
		allInstances: func() ([]instance.Instance, error) {
			return []instance.Instance{
				&mockInstance{id: "one"},
			}, nil
		},
		stopInstances: func(ids []instance.Id) error {
			c.Assert(ids, gc.HasLen, 1)
			c.Assert(ids[0], gc.Equals, instance.Id("one"))
			return nil
		},
		config: configGetter(c),
	}
	err = common.Destroy(env)
	c.Assert(err, gc.IsNil)
	_, err = stor.Get("somewhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *DestroySuite) TestCannotTrashStorageWhenNoInstances(c *gc.C) {
	env := &mockEnviron{
		storage: &mockStorage{removeAllErr: fmt.Errorf("noes!")},
		allInstances: func() ([]instance.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		config: configGetter(c),
	}
	err := common.Destroy(env)
	c.Assert(err, gc.ErrorMatches, "noes!")
}

func (s *DestroySuite) TestSuccessWhenNoInstances(c *gc.C) {
	stor := newStorage(s, c)
	err := stor.Put("elsewhere", strings.NewReader("stuff"), 5)
	c.Assert(err, gc.IsNil)

	env := &mockEnviron{
		storage: stor,
		allInstances: func() ([]instance.Instance, error) {
			return nil, environs.ErrNoInstances
		},
		config: configGetter(c),
	}
	err = common.Destroy(env)
	c.Assert(err, gc.IsNil)
	_, err = stor.Get("elsewhere")
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}
