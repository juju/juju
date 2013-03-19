package state

import "time"

type (
	CharmDoc    charmDoc
	MachineDoc  machineDoc
	RelationDoc relationDoc
	ServiceDoc  serviceDoc
	UnitDoc     unitDoc
)

func (doc *MachineDoc) String() string {
	m := &Machine{doc: machineDoc(*doc)}
	return m.String()
}

var (
	defaultDialTimeout = dialTimeout
defaultRetryDelay = retryDelay
)

func SetDialTimeout(d time.Duration) {
	if d == 0 {
		dialTimeout = defaultDialTimeout
	} else {
		dialTimeout = d
	}
}

func SetRetryDelay(d time.Duration) {
        if d == 0 {
                retryDelay = defaultRetryDelay
        } else {
                retryDelay = d
        }
}


func init() {
	logSize = logSizeTests
}
