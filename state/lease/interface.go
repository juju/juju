package lease

import (
	"time"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2/txn"
)

// Storage exposes lease management functionality on top of MongoDB.
type Storage interface {

	// Refresh reads all lease info for the storage's namespace. You probably
	// don't want to have too many thousand leases in a given namespace.
	Refresh() error

	// Leases returns a recent snapshot of lease state. Expiry times
	// expressed according to the Clock the Storage was configured with.
	Leases() map[string]Info

	// ClaimLease records the supplied holder's claim to the supplied lease. If
	// it succeeds, the claim is guaranteed until at least the supplied duration
	// after the call to ClaimLease was initiated.
	ClaimLease(lease, holder string, duration time.Duration) error

	// ExtendLease records the supplied holder's continued claim to the supplied
	// lease, if necessary. If it succeeds, the claim is guaranteed until at
	// least the supplied duration after the call to ExtendLease was initiated.
	ExtendLease(lease, holder string, duration time.Duration) error

	// ExpireLease records the vacation of the supplied lease. It will fail if
	// we cannot verify that the lease's writer considers the expiry time to
	// have passed.
	ExpireLease(lease string) error
}

// Info is the information a Storage is willing to give out about a given lease.
type Info struct {

	// Holder is the name of the current lease holder.
	Holder string

	// EarliestExpiry is the earliest time at which it's possible the lease
	// might expire.
	EarliestExpiry time.Time

	// LatestExpiry is the latest time at which it's possible the lease might
	// still be valid.
	LatestExpiry time.Time

	// AssertOp is filthy abstraction-breaking garbage that is necessary to
	// allow us to make mgo/txn assertions about leases in the state package;
	// and which thus allows us to gate certain state changes on a particular
	// unit's leadership of a service, for example.
	AssertOp txn.Op
}

// StorageConfig contains the resources and information required to create
// a Storage.
type StorageConfig struct {

	// Clock exposes the wall-clock time to a Storage.
	Clock Clock

	// Mongo exposes the mgo[/txn] capabilities required by a Storage.
	Mongo Mongo

	// Collection identifies the mongodb collection in which lease data is
	// persisted. Multiple storages can use the same collection without
	// interfering with each other, so long as they use different namespaces.
	Collection string

	// Namespace identifies the domain of the lease manager; storage instances
	// which share a namespace and a collection will collaborate.
	Namespace string

	// Instance uniquely identifies the storage instance. Storage instances will
	// fail to collaborate if any two concurrently identify as the same instance.
	Instance string
}

// Validate returns an error if the supplied config is not valid.
func (config StorageConfig) Validate() error {
	return fmt.Errorf("validation!")
}
