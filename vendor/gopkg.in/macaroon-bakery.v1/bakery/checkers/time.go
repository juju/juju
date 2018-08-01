package checkers

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/macaroon.v1"
)

var timeNow = time.Now

// TimeBefore is a checker that checks caveats
// as created by TimeBeforeCaveat.
var TimeBefore = CheckerFunc{
	Condition_: CondTimeBefore,
	Check_: func(_, cav string) error {
		t, err := time.Parse(time.RFC3339Nano, cav)
		if err != nil {
			return errgo.Mask(err)
		}
		if !timeNow().Before(t) {
			return fmt.Errorf("macaroon has expired")
		}
		return nil
	},
}

// TimeBeforeCaveat returns a caveat that specifies that
// the time that it is checked should be before t.
func TimeBeforeCaveat(t time.Time) Caveat {
	return firstParty(CondTimeBefore, t.UTC().Format(time.RFC3339Nano))
}

// ExpiryTime returns the minimum time of any time-before caveats found
// in the given slice and whether there were any such caveats found.
func ExpiryTime(cavs []macaroon.Caveat) (time.Time, bool) {
	var t time.Time
	var expires bool
	for _, cav := range cavs {
		if !strings.HasPrefix(cav.Id, CondTimeBefore) {
			continue
		}
		et, err := time.Parse(CondTimeBefore+" "+time.RFC3339Nano, cav.Id)
		if err != nil {
			continue
		}
		if !expires || et.Before(t) {
			t = et
			expires = true
		}
	}
	return t, expires
}

// MacaroonsExpiryTime returns the minimum time of any time-before
// caveats found in the given macaroons and whether there were
// any such caveats found.
func MacaroonsExpiryTime(ms macaroon.Slice) (time.Time, bool) {
	var t time.Time
	var expires bool
	for _, m := range ms {
		if et, ex := ExpiryTime(m.Caveats()); ex {
			if !expires || et.Before(t) {
				t = et
				expires = true
			}
		}
	}
	return t, expires
}
