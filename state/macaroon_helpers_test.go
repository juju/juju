// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon.v2"
)

func newMacaroon(id string) (*macaroon.Macaroon, error) {
	return macaroon.New(nil, []byte(id), "", macaroon.LatestVersion)
}

func assertMacaroonsEqual(c *gc.C, ms1, ms2 []macaroon.Slice) error {
	if len(ms1) != len(ms2) {
		return errors.Errorf("length mismatch, %d vs %d", len(ms1), len(ms2))
	}

	for i := 0; i < len(ms1); i++ {
		m1 := ms1[i]
		m2 := ms2[i]
		if len(m1) != len(m2) {
			return errors.Errorf("length mismatch, %d vs %d", len(m1), len(m2))
		}
		for i := 0; i < len(m1); i++ {
			assertMacaroonEquals(c, m1[i], m2[i])
		}
	}
	return nil
}

func assertMacaroonEquals(c *gc.C, m1, m2 *macaroon.Macaroon) {
	c.Assert(m1.Id(), jc.DeepEquals, m2.Id())
	c.Assert(m1.Signature(), jc.DeepEquals, m2.Signature())
	c.Assert(m1.Location(), jc.DeepEquals, m2.Location())
}
