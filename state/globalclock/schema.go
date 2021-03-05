// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package globalclock

import (
	"time"

	"github.com/juju/mgo/v2/bson"
)

// clockDocID is the document ID for the global clock document.
const clockDocID = "g"

// clockDoc contains the current global virtual time.
type clockDoc struct {
	DocID string `bson:"_id"`
	Time  int64  `bson:"time"`
}

func (d clockDoc) time() time.Time {
	return time.Unix(0, d.Time)
}

func matchTimeDoc(t time.Time) bson.D {
	return bson.D{
		{"_id", clockDocID},
		{"time", t.UnixNano()},
	}
}

func setTimeDoc(t time.Time) bson.D {
	return bson.D{{"$set", bson.D{{"time", t.UnixNano()}}}}
}
