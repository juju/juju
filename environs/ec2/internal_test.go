package ec2

import (
	"errors"
	. "launchpad.net/gocheck"
	"launchpad.net/goamz/ec2"
	"time"
)

type internalSuite struct{}

var _ = Suite(internalSuite{})

var samePermsTests = []struct {
	same bool
	p0   []ec2.IPPerm
	p1   []ec2.IPPerm
}{
	{true,
		nil, nil},
	{false,
		nil, []ec2.IPPerm{{}}},
	{true,
		[]ec2.IPPerm{{}}, []ec2.IPPerm{{}}},
	{true,
		[]ec2.IPPerm{{
			Protocol:  "a",
			FromPort:  1,
			ToPort:    2,
			SourceIPs: []string{"y", "x"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "ignored0",
				OwnerId: "ignored0also",
				Name:    "g1",
			}},
		}, {
			Protocol:  "b",
			FromPort:  5,
			ToPort:    6,
			SourceIPs: []string{"w", "z"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "ignored1",
				OwnerId: "ignored1also",
				Name:    "g1",
			}, {
				Id:      "ignored2",
				OwnerId: "ignored2also",
				Name:    "g2",
			}},
		}},
		[]ec2.IPPerm{{
			Protocol:  "b",
			FromPort:  5,
			ToPort:    6,
			SourceIPs: []string{"z", "w"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "other0",
				OwnerId: "other1",
				Name:    "g2",
			}, {
				Id:      "ignored1",
				OwnerId: "ignored1also",
				Name:    "g1",
			}},
		}, {
			Protocol:  "a",
			FromPort:  1,
			ToPort:    2,
			SourceIPs: []string{"x", "y"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "other2",
				OwnerId: "other3",
				Name:    "g1",
			}},
		}}},
	{false,
		[]ec2.IPPerm{{
			Protocol:  "b",
			FromPort:  5,
			ToPort:    6,
			SourceIPs: []string{"w", "z"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "ignored1",
				OwnerId: "ignored1also",
				Name:    "g1",
			}, {
				Id:      "ignored2",
				OwnerId: "ignored2also",
				Name:    "g2",
			}},
		}},
		[]ec2.IPPerm{{
			Protocol:  "b",
			FromPort:  5,
			ToPort:    6,
			SourceIPs: []string{"w", "z"},
			SourceGroups: []ec2.UserSecurityGroup{{
				Id:      "ignored2",
				OwnerId: "ignored2also",
				Name:    "g2",
			}},
		}}}}


// copyPerms makes a copy of the permissions
// so that samePerms won't change the original.
func copyPerms(ps []ec2.IPPerm) []ec2.IPPerm {
	rs := make([]ec2.IPPerm, len(ps))
	for i, p := range ps {
		r := &rs[i]
		*r = p
		r.SourceIPs = make([]string, len(p.SourceIPs))
		copy(r.SourceIPs, p.SourceIPs)
		r.SourceGroups = make([]ec2.UserSecurityGroup, len(p.SourceGroups))
		copy(r.SourceGroups, p.SourceGroups)
	}
	return rs
}

func (internalSuite) TestSamePerms(c *C) {
	for i, t := range samePermsTests {
		m := samePerms(copyPerms(t.p0), copyPerms(t.p1))
		if m != t.same {
			c.Errorf("test %d, expected %v got %v", i, t.same, m)
		}
		m = samePerms(copyPerms(t.p1), copyPerms(t.p0))
		if m != t.same {
			c.Errorf("test %d reversed, expected %v got %v", i, t.same, m)
		}
	}
}

func (internalSuite) TestAttemptTiming(c *C) {
	const delta = 0.01e9
	a := attempt{
		0.25e9, 0.1e9,
		0.4e9, 0.15e9,
	}
	want := []time.Duration{0, 0.1e9, 0.2e9, 0.3e9,  0.45e9, 0.60e9}
	got := make([]time.Duration, 0, len(want))	// avoid allocation when testing timing
	t0 := time.Now()
	a.do(errorIs(err1), func() error {
		got = append(got, time.Now().Sub(t0))
		return err1
	})
	t1 := time.Now()
	c.Assert(len(got), Equals, len(want), Bug("got %v", got))
	for i, got := range want {
		lo := want[i] - delta
		hi := want[i] + delta
		if got < lo || got > hi {
			c.Errorf("attempt %d want %g got %g", i, want[i].Seconds(), got.Seconds())
		}
	}
	max := a.burstTotal + a.longTotal + a.burstDelay + a.longDelay + delta
	actual := t1.Sub(t0)
	if actual > max {
		c.Errorf("total time exceeded, want less than %gs got %gs", max.Seconds(), actual.Seconds())
	}
}

func errorGen(n *int, errs ...error) func() error {
	return func() error {
		if n != nil {
			(*n)++
		}
		if len(errs) == 1 {
			// when we've got to the end, don't increment iteration count
			// any more, so we can check against an indefinite number
			// of iterations.
			n = nil
			return errs[0]
		}
		err := errs[0]
		errs = errs[1:]
		return err
	}
}

var err1 = errors.New("one")
var err2 = errors.New("two")

func errorIs(err error) func(error) bool {
	return func(e error) bool {
		return e == err
	}
}

func never(error) bool {
	return false
}

func (internalSuite) TestAttemptError(c *C) {
	var iter int

	tests := []struct{
		transient func(error) bool
		f func() error
		err error
		iter int
	} {{
		never,
		errorGen(&iter, nil),
		nil,
		1,
	}, {
		errorIs(err1),
		errorGen(&iter, err1, err1, err2),
		err2,
		3,
	}, {
		errorIs(err1),
		errorGen(&iter, err1, err1, nil),
		nil,
		3,
	}, {
		never,
		errorGen(&iter, err2),
		err2,
		1,
	}, {
		errorIs(err1),
		errorGen(&iter, err1, err1, err1),
		err1,
		3,
	}}

	a := attempt{
		0.1e9,
		0.02e9,
		0,
		0,
	}

	for i, t := range tests {
		iter = 0
		err := a.do(t.transient, t.f)
		c.Check(err, Equals, t.err, Bug("test %d", i))
		c.Check(iter, Equals, t.iter, Bug("test %d", i))
	}
}
