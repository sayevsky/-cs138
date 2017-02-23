package tapestry

import (
	"fmt"
	"net"
	"os"
	"time"
)

/* The Tapestry object provides the API for accessing tapestry.
 * It will call remote methods across RPC, and receives remote RPC
 * calls which get forwarded to the local node */

const BASE = 16    // The base of a digit of an ID.  By default, a digit is base-16
const DIGITS = 40  // The number of digits in an ID.  By default, an ID has 40 digits.
const RETRIES = 3  // The number of retries on failure. By default we have 3 retries
const K = 10       // During neighbour traversal, trim the neighbourset to this size before fetching backpointers. By default this has a value of 10
const SLOTSIZE = 3 // The each slot in the routing table should store this many nodes.  By default this is 3.

const REPUBLISH = 10 * time.Second // object republish interval for nodes advertising objects
const TIMEOUT = 25 * time.Second   // object timeout interval for nodes storing objects

/*
	Provides the private API for communicating with remote nodes
*/
type Tapestry struct {
	local     *TapestryNode      // the local node
	server    *TapestryRPCServer // receives remote method invocations and calls the corresponding local node methods
	blobstore *BlobStore         // stores blobs on the local node
}

/*
	Public API: Start a tapestry node on the specified port.

	Optionally, specify the address of an existing node in the tapestry mesh to connect to, otherwise set to ""
*/
func Start(port int, connectTo string) (*Tapestry, error) {
	return start(RandomID(), port, connectTo)
}

/*
	Private method, useful for testing: start a node with the specified ID rather than a random ID
*/
func start(id ID, port int, connectTo string) (tapestry *Tapestry, err error) {
	// Create the tapestry object
	tapestry = new(Tapestry)

	// Create the blob store
	tapestry.blobstore = NewBlobStore()

	// Create the RPC server
	tapestry.server, err = newTapestryRPCServer(port, tapestry)
	if err != nil {
		return nil, err
	}

	// Get the hostname of this machine
	name, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("Unable to get hostname of local machine to start Tapestry node. Reason: %v", err)
	}

	// Get the port we are bound to
	_, actualport, err := net.SplitHostPort(tapestry.server.listener.Addr().String()) //fmt.Sprintf("%v:%v", name, port)
	if err != nil {
		return nil, err
	}

	// The actual address of this node
	address := fmt.Sprintf("%s:%s", name, actualport)

	// Create the local node
	tapestry.local = newTapestryNode(Node{id, address}, tapestry)

	// If specified, connect to the provided address
	if connectTo != "" {
		// Get the node we're joining
		node, err := tapestry.hello(connectTo)
		if err != nil {
			return nil, fmt.Errorf("Error joining existing tapestry node %v, reason: %v", address, err)
		}
		err = tapestry.local.Join(node)
		if err != nil {
			return nil, err
		}
	}

	return tapestry, nil
}

/*
	Leave the tapestry.  In the current implementation, this method may only be called once
*/
func (tapestry *Tapestry) Leave() {
	tapestry.local.Leave()
	tapestry.server.listener.Close()
}

/*
   Kill this node without gracefully leaving the tapestry
*/
func (tapestry *Tapestry) Kill() {
	tapestry.server.listener.Close()
}

/*
	Store a blob on the local node and publish the key to the tapestry
*/
func (tapestry *Tapestry) Store(key string, value []byte) error {
	done, err := tapestry.local.Publish(key)
	if err != nil {
		return err
	}
	tapestry.blobstore.Put(key, value, done)
	return nil
}

/*
	Lookup a key in the tapestry and return its root node
*/
func (tapestry *Tapestry) Lookup(key string) ([]Node, error) {
	return tapestry.local.Lookup(key)
}

/*
	Lookup a key in the tapestry then fetch the corresponding blob from the remote blob store
*/
func (tapestry *Tapestry) Get(key string) ([]byte, error) {
	// Lookup the key
	replicas, err := tapestry.Lookup(key)
	if err != nil {
		return nil, err
	}
	if len(replicas) == 0 {
		return nil, fmt.Errorf("No replicas returned for key %v", key)
	}

	// Contact replicas
	var errs []error
	for _, replica := range replicas {
		blob, err := FetchRemoteBlob(replica, key)
		if err != nil {
			errs = append(errs, err)
		}
		if blob != nil {
			return *blob, nil
		}
	}

	return nil, fmt.Errorf("Error contacting replicas, %v: %v", replicas, errs)
}

/*
	Remove the blob from the local blob store and stop advertising
*/
func (tapestry *Tapestry) Remove(key string) bool {
	return tapestry.blobstore.Delete(key)
}

func (tapestry *Tapestry) String() string {
	return fmt.Sprintf("Tapestry Node %v at %v", tapestry.local.node.Id, tapestry.local.node.Address)
}
