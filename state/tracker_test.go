package state

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type TrackerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&TrackerSuite{})

var _ = jc.Satisfies

func (*TrackerSuite) TestAddsReference(c *gc.C) {
	tracker := newTracker()
	tracker.Add(&State{})
	c.Assert(tracker.count(), gc.Equals, 1)
}

// Unfortunately there is no deterministic way to test the finalizer as the GC
// will just schedule it to run at some arbitrary time after obj becomes
// unreachable.

func (*TrackerSuite) TestReport(c *gc.C) {
	tracker := newTracker()
	tracker.Add(&State{})
	closed := &State{}
	tracker.Add(closed)
	tracker.RecordClosed(closed)
	tracker.Add(&State{})

	report := tracker.IntrospectionReport()
	// The closed models are at the end.
	c.Assert(report, gc.Matches, `(?ms)Total count: 3.Closed count: 1.`+
		`.*Model:.*`+
		`.*Model:.*`+
		`.*Model:.*Closed: .*`)
}
