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

var oldDialTimeout = dialTimeout

func SetDialTimeout(d time.Duration) {
	if d == 0 {
		dialTimeout = oldDialTimeout
	} else {
		dialTimeout = d
	}
}

func init() {
	logSize = logSizeTests
}
