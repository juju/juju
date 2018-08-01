package mgostorage

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/macaroon-bakery.v2-unstable/bakery"
)

// Functions defined as variables so they can be overidden
// for testing.
var (
	timeNow = time.Now

	mgoCollectionFindId = (*mgo.Collection).FindId
)

var logger = loggo.GetLogger("bakery.mgostorage")

// maxPolicyCache holds the maximum number of storage policies that can
// hold cached keys in a given RootKeys instance.
//
// 100 is probably overkill, given that practical systems will
// likely only have a small number of active policies on any given
// macaroon collection.
const maxPolicyCache = 100

// RootKeys represents a cache of macaroon root keys.
type RootKeys struct {
	maxCacheSize int

	// TODO (rogpeppe) use RWMutex instead of Mutex here so that
	// it's faster in the probably-common case that we
	// have many contended readers.
	mu       sync.Mutex
	oldCache map[string]rootKey
	cache    map[string]rootKey

	// current holds the current root key for each storage policy.
	current map[Policy]rootKey
}

type rootKey struct {
	Id      []byte `bson:"_id"`
	Created time.Time
	Expires time.Time
	RootKey []byte
}

// isValid reports whether the root key contains a key. Note that we
// always generate non-empty root keys, so we use this to find
// whether the root key is empty or not.
func (rk rootKey) isValid() bool {
	return rk.RootKey != nil
}

// isValidWithPolicy reports whether the given root key
// is currently valid to use with the given storage policy.
func (rk rootKey) isValidWithPolicy(p Policy) bool {
	if !rk.isValid() {
		return false
	}
	now := timeNow()
	return afterEq(rk.Created, now.Add(-p.GenerateInterval)) &&
		afterEq(rk.Expires, now.Add(p.ExpiryDuration)) &&
		beforeEq(rk.Expires, now.Add(p.ExpiryDuration+p.GenerateInterval))
}

// NewRootKeys returns a root-keys cache that
// is limited in size to approximately the given size.
//
// The NewStorage method returns a storage implementation
// that uses a specific mongo collection and storage
// policy.
func NewRootKeys(maxCacheSize int) *RootKeys {
	return &RootKeys{
		maxCacheSize: maxCacheSize,
		cache:        make(map[string]rootKey),
		current:      make(map[Policy]rootKey),
	}
}

// Policy holds a storage policy for root keys.
type Policy struct {
	// GenerateInterval holds the maximum length of time
	// for which a root key will be returned from RootKey.
	// If this is zero, it defaults to ExpiryDuration.
	GenerateInterval time.Duration

	// ExpiryDuration holds the minimum length of time that
	// root keys will be valid for after they are returned from
	// RootKey. The maximum length of time that they
	// will be valid for is ExpiryDuration + GenerateInterval.
	ExpiryDuration time.Duration
}

// NewStorage returns a new RootKeyStorage implementation that
// stores and obtains root keys from the given collection.
//
// Root keys will be generated and stored following the
// given storage policy.
//
// It is expected that all collections passed to a given Storage's
// NewStorage method should refer to the same underlying collection.
func (s *RootKeys) NewStorage(c *mgo.Collection, policy Policy) bakery.Storage {
	if policy.GenerateInterval == 0 {
		policy.GenerateInterval = policy.ExpiryDuration
	}
	return &storage{
		keys:   s,
		coll:   c,
		policy: policy,
	}
}

var indexes = []mgo.Index{{
	Key: []string{"-created"},
}, {
	Key:         []string{"expires"},
	ExpireAfter: time.Second,
}}

// EnsureIndex ensures that the required indexes exist on the
// collection that will be used for root key storage.
// This should be called at least once before using NewStorage.
func (s *RootKeys) EnsureIndex(c *mgo.Collection) error {
	for _, idx := range indexes {
		if err := c.EnsureIndex(idx); err != nil {
			return errgo.Notef(err, "cannot ensure index for %q on %q", idx.Key, c.Name)
		}
	}
	return nil
}

// get gets the root key for the given id, trying the cache first and
// falling back to calling fallback if it's not found there.
//
// If the key does not exist or has expired, it returns
// bakery.ErrNotFound.
//
// Called with s.mu locked.
func (s *RootKeys) get(id []byte, fallback func(id []byte) (rootKey, error)) (rootKey, error) {
	key, cached, err := s.get0(id, fallback)
	if err != nil && err != bakery.ErrNotFound {
		return rootKey{}, errgo.Mask(err)
	}
	if err == nil && timeNow().After(key.Expires) {
		key = rootKey{}
		err = bakery.ErrNotFound
	}
	if !cached {
		s.addCache(id, key)
	}
	return key, err
}

// get0 is the inner version of RootKeys.get. It returns an item and reports
// whether it was found in the cache, but doesn't check whether the
// item has expired or move the returned item to s.cache.
func (s *RootKeys) get0(id []byte, fallback func(id []byte) (rootKey, error)) (key rootKey, inCache bool, err error) {
	if k, ok := s.cache[string(id)]; ok {
		if !k.isValid() {
			return rootKey{}, true, bakery.ErrNotFound
		}
		return k, true, nil
	}
	if k, ok := s.oldCache[string(id)]; ok {
		if !k.isValid() {
			return rootKey{}, false, bakery.ErrNotFound
		}
		return k, false, nil
	}
	logger.Infof("cache miss for %q", id)
	k, err := fallback(id)
	return k, false, err
}

// addCache adds the given key to the cache.
// Called with s.mu locked.
func (s *RootKeys) addCache(id []byte, k rootKey) {
	if len(s.cache) >= s.maxCacheSize {
		s.oldCache = s.cache
		s.cache = make(map[string]rootKey)
	}
	s.cache[string(id)] = k
}

// setCurrent sets the current key for the given storage policy.
// Called with s.mu locked.
func (s *RootKeys) setCurrent(policy Policy, key rootKey) {
	if len(s.current) > maxPolicyCache {
		// Sanity check to avoid possibly memory leak:
		// if some client is using arbitrarily many storage
		// policies, we don't want s.keys.current to endlessly
		// expand, so just kill the cache if it grows too big.
		// This will result in worse performance but it shouldn't
		// happen in practice and it's better than using endless
		// space.
		s.current = make(map[Policy]rootKey)
	}
	s.current[policy] = key
}

type storage struct {
	keys   *RootKeys
	policy Policy
	coll   *mgo.Collection
}

// Get implements bakery.RootKeyStorage.Get.
func (s *storage) Get(id []byte) ([]byte, error) {
	s.keys.mu.Lock()
	defer s.keys.mu.Unlock()

	key, err := s.keys.get(id, s.getFromMongo)
	if err != nil {
		return nil, err
	}
	return key.RootKey, nil
}

func (s *storage) getFromMongo(id []byte) (rootKey, error) {
	var key rootKey
	err := mgoCollectionFindId(s.coll, id).One(&key)
	if err != nil {
		if err == mgo.ErrNotFound {
			return s.getLegacyFromMongo(string(id))
		}
		return rootKey{}, errgo.Notef(err, "cannot get key from database")
	}
	// TODO migrate the key from the old format to the new format.
	return key, nil
}

// getLegacyFromMongo gets a value from the old version of the
// root key document which used a string key rather than a []byte
// key.
func (s *storage) getLegacyFromMongo(id string) (rootKey, error) {
	var key rootKey
	err := mgoCollectionFindId(s.coll, id).One(&key)
	if err != nil {
		if err == mgo.ErrNotFound {
			return rootKey{}, bakery.ErrNotFound
		}
		return rootKey{}, errgo.Notef(err, "cannot get key from database")
	}
	return key, nil
}

// RootKey implements bakery.RootKeyStorage.RootKey by
// returning an existing key from the cache when compatible
// with the current policy.
func (s *storage) RootKey() ([]byte, []byte, error) {
	if key := s.rootKeyFromCache(); key.isValid() {
		return key.RootKey, key.Id, nil
	}
	logger.Debugf("root key cache miss")
	// Try to find a root key from the collection.
	// It doesn't matter much if two concurrent mongo
	// clients are doing this at the same time because
	// we don't mind if there are more keys than necessary.
	//
	// Note that this query mirrors the logic found in
	// storage.rootKeyFromCache.
	now := timeNow()
	var key rootKey
	err := s.coll.Find(bson.D{{
		"created", bson.D{{"$gte", now.Add(-s.policy.GenerateInterval)}},
	}, {
		"expires", bson.D{
			{"$gte", now.Add(s.policy.ExpiryDuration)},
			{"$lte", now.Add(s.policy.ExpiryDuration + s.policy.GenerateInterval)},
		},
	}}).Sort("-created").One(&key)
	if err != nil && err != mgo.ErrNotFound {
		return nil, nil, errgo.Notef(err, "cannot query existing keys")
	}
	if !key.isValid() {
		// No keys found anywhere, so let's create one.
		var err error
		key, err = s.generateKey()
		if err != nil {
			return nil, nil, errgo.Notef(err, "cannot generate key")
		}
		logger.Infof("new root key id %q; created %v; expires %v", key.Id, key.Created, key.Expires)
		if err := s.coll.Insert(key); err != nil {
			return nil, nil, errgo.Notef(err, "cannot create root key")
		}
	}
	s.keys.mu.Lock()
	defer s.keys.mu.Unlock()
	s.keys.addCache(key.Id, key)
	s.keys.setCurrent(s.policy, key)
	return key.RootKey, key.Id, nil
}

// rootKeyFromCache returns a root key from the cached keys.
// If no keys are found that are valid for s.policy, it returns
// the zero key.
func (s *storage) rootKeyFromCache() rootKey {
	s.keys.mu.Lock()
	defer s.keys.mu.Unlock()
	if k, ok := s.keys.current[s.policy]; ok && k.isValidWithPolicy(s.policy) {
		return k
	}

	// Find the most recently created key that's consistent with the
	// storage policy.
	var current rootKey
	for _, k := range s.keys.cache {
		if k.isValidWithPolicy(s.policy) && k.Created.After(current.Created) {
			current = k
		}
	}
	if current.isValid() {
		s.keys.current[s.policy] = current
		return current
	}
	return rootKey{}
}

func (s *storage) generateKey() (rootKey, error) {
	newKey, err := randomBytes(24)
	if err != nil {
		return rootKey{}, err
	}
	newId, err := randomBytes(16)
	if err != nil {
		return rootKey{}, err
	}
	now := timeNow()
	return rootKey{
		Created: now,
		Expires: now.Add(s.policy.ExpiryDuration + s.policy.GenerateInterval),
		// TODO return just newId when we know we can always
		// use non-text macaroon ids.
		Id:      []byte(fmt.Sprintf("%x", newId)),
		RootKey: newKey,
	}, nil
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, fmt.Errorf("cannot generate %d random bytes: %v", n, err)
	}
	return b, nil
}

// afterEq reports whether t0 is after or equal to t1.
func afterEq(t0, t1 time.Time) bool {
	return !t0.Before(t1)
}

// beforeEq reports whether t1 is before or equal to t0.
func beforeEq(t0, t1 time.Time) bool {
	return !t0.After(t1)
}
