package bakery

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// Storage defines storage for macaroons.
// Calling its methods concurrently is allowed.
//
// This type is the original storage interface supported
// by the bakery and will eventually be deprecated
// in favour of RootKeyStorage.
type Storage interface {
	// Put stores the item at the given location, overwriting
	// any item that might already be there.
	// TODO(rog) would it be better to lose the overwrite
	// semantics?
	Put(location string, item string) error

	// Get retrieves an item from the given location.
	// If the item is not there, it returns ErrNotFound.
	Get(location string) (item string, err error)

	// Del deletes the item from the given location.
	Del(location string) error
}

// RootKeyStorage defines defines storage for macaroon
// root keys.
type RootKeyStorage interface {
	// Get returns the root key for the given id.
	// If the item is not there, it returns ErrNotFound.
	Get(id string) ([]byte, error)

	// RootKey returns the root key to be used for making a new
	// macaroon, and an id that can be used to look it up later with
	// the Get method.
	//
	// Note that the root keys should remain available for as long
	// as the macaroons using them are valid.
	//
	// Note that there is no need for it to return a new root key
	// for every call - keys may be reused, although some key
	// cycling is over time is advisable.
	RootKey() (rootKey []byte, id string, err error)
}

// ErrNotFound is returned by Storage.Get implementations
// to signal that an id has not been found.
var ErrNotFound = errors.New("item not found")

// NewMemRootKeyStorage returns an implementation of
// RootKeyStorage that generates a single key and always
// returns that from RootKey. The same id ("0") is always
// used.
func NewMemRootKeyStorage() RootKeyStorage {
	return new(memRootKeyStorage)
}

type memRootKeyStorage struct {
	mu  sync.Mutex
	key []byte
}

// Get implements RootKeyStorage.Get.
func (s *memRootKeyStorage) Get(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id != "0" || s.key == nil {
		return nil, ErrNotFound
	}
	return s.key, nil
}

// RootKey implements RootKeyStorage.RootKey by
//always returning the same root key.
func (s *memRootKeyStorage) RootKey() (rootKey []byte, id string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key == nil {
		newKey, err := randomBytes(24)
		if err != nil {
			return nil, "", err
		}
		s.key = newKey
	}
	return s.key, "0", nil
}

// NewMemStorage returns an implementation of Storage
// that stores all items in memory.
func NewMemStorage() Storage {
	return &memStorage{
		values: make(map[string]string),
	}
}

type memStorage struct {
	mu     sync.Mutex
	values map[string]string
}

func (s *memStorage) Put(location, item string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[location] = item
	return nil
}

func (s *memStorage) Get(location string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.values[location]
	if !ok {
		return "", ErrNotFound
	}
	return item, nil
}

func (s *memStorage) Del(location string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, location)
	return nil
}

// storageItem is the format used to store items in
// the store.
type storageItem struct {
	RootKey []byte
}

// storage is a thin wrapper around Storage that
// converts to and from []byte root keys in its
// Put and Get methods.
type storage struct {
	store Storage
}

func (s storage) Get(location string) ([]byte, error) {
	itemStr, err := s.store.Get(location)
	if err != nil {
		return nil, err
	}
	var item storageItem
	if err := json.Unmarshal([]byte(itemStr), &item); err != nil {
		return nil, fmt.Errorf("badly formatted item in store: %v", err)
	}
	return item.RootKey, nil
}

func (s storage) Put(location string, key []byte) error {
	item := &storageItem{
		RootKey: key,
	}
	data, err := json.Marshal(item)
	if err != nil {
		panic(fmt.Errorf("cannot marshal storage item: %v", err))
	}
	return s.store.Put(location, string(data))
}
