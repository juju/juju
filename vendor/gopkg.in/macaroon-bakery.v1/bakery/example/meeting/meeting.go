package meeting

import (
	"crypto/rand"
	"fmt"
	"sync"
)

type Place struct {
	mu    sync.Mutex
	items map[string]*item
}

type item struct {
	c     chan struct{}
	data0 []byte
	data1 []byte
}

func New() *Place {
	return &Place{
		items: make(map[string]*item),
	}
}

func newId() (string, error) {
	var id [12]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("cannot read random id: %v", err)
	}
	return fmt.Sprintf("%x", id[:]), nil
}

func (m *Place) NewRendezvous(data []byte) (string, error) {
	id, err := newId()
	if err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[id] = &item{
		c:     make(chan struct{}),
		data0: data,
	}
	return id, nil
}

func (m *Place) Wait(id string) (data0, data1 []byte, err error) {
	m.mu.Lock()
	item := m.items[id]
	m.mu.Unlock()
	if item == nil {
		return nil, nil, fmt.Errorf("rendezvous %q not found", id)
	}
	<-item.c
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, id)
	return item.data0, item.data1, nil
}

func (m *Place) Done(id string, data []byte) error {
	m.mu.Lock()
	item := m.items[id]
	defer m.mu.Unlock()

	if item == nil {
		return fmt.Errorf("rendezvous %q not found", id)
	}
	select {
	case <-item.c:
		return fmt.Errorf("rendezvous %q done twice", id)
	default:
		item.data1 = data
		close(item.c)
	}
	return nil
}
