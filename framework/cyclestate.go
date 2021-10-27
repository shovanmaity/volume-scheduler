package framework

import (
	"sync"

	"github.com/pkg/errors"
)

var (
	// ErrNotFound is the not found error message.
	ErrNotFound = errors.New("not found")
)

// StateData is a generic type for arbitrary data stored in CycleState.
type StateData interface {
	// Clone is an interface to make a copy of StateData. For performance reasons,
	// clone should make shallow copies for members (e.g., slices or maps).
	Clone() StateData
}

// StateKey is the type of keys stored in CycleState.
type StateKey string

// CycleState provides a mechanism for plugins to store and retrieve arbitrary data.
// StateData stored by one plugin can be read, altered, or deleted by another plugin.
// CycleState does not provide any data protection.
type CycleState struct {
	mx      sync.RWMutex
	storage map[StateKey]StateData
}

// NewCycleState initializes a new CycleState and returns its pointer.
func NewCycleState() *CycleState {
	return &CycleState{
		storage: make(map[StateKey]StateData),
	}
}

// Clone creates a copy of CycleState and returns its pointer. Clone returns
// nil if the context being cloned is nil.
func (c *CycleState) Clone() *CycleState {
	if c == nil {
		return nil
	}
	copy := NewCycleState()
	for k, v := range c.storage {
		copy.Write(k, v.Clone())
	}
	return copy
}

// Read retrieves data with the given "key" from CycleState. If the key is not
// present an error is returned, this function is thread safe.
func (c *CycleState) Read(key StateKey) (StateData, error) {
	c.mx.RLock()
	defer c.mx.RUnlock()
	if v, ok := c.storage[key]; ok {
		return v, nil
	}
	return nil, ErrNotFound
}

// Write stores the given "val" in CycleState with the given "key", this function
// is thread safe..
func (c *CycleState) Write(key StateKey, val StateData) {
	c.mx.Lock()
	c.storage[key] = val
	c.mx.Unlock()
}

// Delete deletes data with the given "key" from CycleState, this function is thread safe.
func (c *CycleState) Delete(key StateKey) {
	c.mx.Lock()
	delete(c.storage, key)
	c.mx.Unlock()
}
