package tapestry

import (
	"sync"
	"time"
)

/*
	Objects advertised to the tapestry get stored in the object store of the object's root node.

	An object can be advertised by multiple nodes

	Objects time out after some amount of time if the advertising node is not heard from
*/
type ObjectStore struct {
	mutex sync.Mutex                      // to manage concurrent access to the object store
	data  map[string]map[Node]*time.Timer // multimap: stores multiple nodes per key, and each node has a timeout
}

/*
	Create a new objectstore
*/
func NewObjectStore() *ObjectStore {
	m := new(ObjectStore)
	m.data = make(map[string]map[Node]*time.Timer)
	return m
}

/*
	Registers the specified node as having advertised the key.  Times out after the specified duration.
*/
func (store *ObjectStore) Register(key string, replica Node, timeout time.Duration) bool {
	store.mutex.Lock()

	// Get the value set for the object
	_, exists := store.data[key]
	if !exists {
		store.data[key] = make(map[Node]*time.Timer)
	}

	// Add the value to the value set
	timer, exists := store.data[key][replica]
	if !exists {
		store.data[key][replica] = store.newTimeout(key, replica, timeout)
	} else {
		timer.Reset(TIMEOUT)
	}

	store.mutex.Unlock()

	return !exists
}

/*
	Registers all of the provided nodes and keys.
*/
func (store *ObjectStore) RegisterAll(replicamap map[string][]Node, timeout time.Duration) {
	store.mutex.Lock()

	for key, replicas := range replicamap {
		_, exists := store.data[key]
		if !exists {
			store.data[key] = make(map[Node]*time.Timer)
		}
		for _, replica := range replicas {
			store.data[key][replica] = store.newTimeout(key, replica, timeout)
		}
	}

	store.mutex.Unlock()
}

/*
	Unregisters the specified node for the specified key.  Returns false if the node was not registered for the key.
*/
func (store *ObjectStore) Unregister(key string, replica Node) bool {
	store.mutex.Lock()

	_, existed := store.data[key][replica]
	delete(store.data[key], replica)

	store.mutex.Unlock()

	return existed
}

/*
	Unregisters all nodes that are registered for the provided key.  Returns all replicas that were advertising the key.
*/
func (store *ObjectStore) UnregisterAll(key string) (replicas []Node) {
	store.mutex.Lock()

	replicas = slice(store.data[key])
	delete(store.data, key)

	store.mutex.Unlock()

	return
}

/*
	Get the nodes that are advertising a given key
*/
func (store *ObjectStore) Get(key string) (replicas []Node) {
	store.mutex.Lock()

	replicas = slice(store.data[key])

	store.mutex.Unlock()

	return
}

/*
	Removes and returns all objects that should be transferred to the remote node
*/
func (store *ObjectStore) GetTransferRegistrations(local Node, remote Node) map[string][]Node {
	transfer := make(map[string][]Node)

	store.mutex.Lock()

	for key, values := range store.data {
		// Compare the first digit after the prefix
		if Hash(key).BetterChoice(remote.Id, local.Id) {
			transfer[key] = slice(values)
		}
	}

	for key, _ := range transfer {
		delete(store.data, key)
	}

	store.mutex.Unlock()

	return transfer
}

/*
   Utility method. Creates an expiry timer for the (key, value) pair.
*/
func (store *ObjectStore) newTimeout(key string, replica Node, timeout time.Duration) *time.Timer {
	expire := func() {
		Debug.Printf("Expiring %v for node %v\n", key, replica)

		store.mutex.Lock()

		timer, exists := store.data[key][replica]
		if exists {
			timer.Stop()
			delete(store.data[key], replica)
		}

		store.mutex.Unlock()
	}

	return time.AfterFunc(timeout, expire)
}

// Utility function to get the keys of a map
func slice(valmap map[Node]*time.Timer) (values []Node) {
	for value, _ := range valmap {
		values = append(values, value)
	}
	return
}
