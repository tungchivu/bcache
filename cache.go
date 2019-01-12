package bcache

import (
	"sync"
	"time"

	"github.com/hashicorp/golang-lru"
	"github.com/weaveworks/mesh"
)

type cache struct {
	peerID mesh.PeerName
	mux    sync.RWMutex
	cc     *lru.Cache
}

func newCache(maxKeys int) (*cache, error) {
	cc, err := lru.New(maxKeys)
	if err != nil {
		return nil, err
	}

	return &cache{
		cc: cc,
	}, nil
}

// value represent cache value
type value struct {
	value   string
	expired int64
}

// Set sets the value of a cache
func (c *cache) Set(key, val string, expiredTimestamp int64) {
	c.cc.Add(key, value{
		value:   val,
		expired: expiredTimestamp,
	})

}

// Get gets cache value of the given key
func (c *cache) get(key string) (string, int64, bool) {
	cacheVal, ok := c.cc.Get(key)
	if !ok {
		return "", 0, false
	}
	val := cacheVal.(value)
	return val.value, val.expired, true
}

// Get gets cache value of the given key
func (c *cache) Get(key string) (string, bool) {
	val, expired, ok := c.get(key)
	if !ok {
		return "", false
	}
	// check for expiration
	if expired > 0 && time.Now().Unix() > expired {
		c.cc.Remove(key)
		return "", false
	}

	return val, true

}

// merges received data into state and returns a
// representation of the received data (typically a delta) for further
// propagation.
func (c *cache) mergeDelta(msg *message) (delta mesh.GossipData) {
	delta, _ = c.mergeChange(msg)
	return delta
}

// merges received data into state and returns "everything new
// I've just learnt", or nil if nothing in the received data was new.
func (c *cache) mergeNew(msg *message) (delta mesh.GossipData) {
	var changedKey int
	delta, changedKey = c.mergeChange(msg)
	if changedKey == 0 {
		return nil
	}
	return delta
}

func (c *cache) mergeChange(msg *message) (delta mesh.GossipData, changedKey int) {
	c.mux.Lock()
	defer c.mux.Unlock()

	if len(msg.Entries) == 0 {
		return
	}

	var existingKeys []string
	for _, e := range msg.Entries {
		val, ok := c.cc.Get(e.Key)
		if ok && val == e.Val {
			existingKeys = append(existingKeys, e.Key)
			continue
		}
		c.Set(e.Key, e.Val, e.Expired)
		changedKey++
	}

	// delete key that already existed in this cache
	for _, key := range existingKeys {
		delete(msg.Entries, key)
	}

	return newMessageFromEntries(c.peerID, msg.Entries), changedKey
}
