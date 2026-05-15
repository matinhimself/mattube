package api

import (
	"bytes"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	thumbShards      = 32
	thumbBufInitCap  = 32 * 1024
	thumbBufMaxKeep  = 256 * 1024
	thumbMaxItemSize = 5 << 20
)

// thumbEntry is a cached thumbnail payload.
type thumbEntry struct {
	data        []byte
	contentType string
	expiresAt   time.Time
}

type thumbShard struct {
	mu      sync.RWMutex
	entries map[string]*thumbEntry
	bytes   int64
}

// ThumbCache is a sharded, byte-bounded in-memory cache for thumbnails
// fetched through the fronting transport. It hands out reusable
// *bytes.Buffer instances via sync.Pool so the hot fetch path avoids
// per-request allocation.
type ThumbCache struct {
	shards   [thumbShards]*thumbShard
	perShard int64
	ttl      time.Duration
	bufPool  sync.Pool

	hits   atomic.Int64
	misses atomic.Int64
	stores atomic.Int64
	evicts atomic.Int64
}

// NewThumbCache creates a cache budgeted to maxBytes total across all
// shards, with entries expiring after ttl.
func NewThumbCache(maxBytes int64, ttl time.Duration) *ThumbCache {
	if maxBytes <= 0 {
		maxBytes = 64 << 20 // 64 MiB
	}
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	c := &ThumbCache{
		perShard: maxBytes / thumbShards,
		ttl:      ttl,
		bufPool: sync.Pool{
			New: func() any { return bytes.NewBuffer(make([]byte, 0, thumbBufInitCap)) },
		},
	}
	for i := range c.shards {
		c.shards[i] = &thumbShard{entries: make(map[string]*thumbEntry, 256)}
	}
	return c
}

func (c *ThumbCache) shardFor(key string) *thumbShard {
	h := fnv.New32a()
	h.Write([]byte(key))
	return c.shards[h.Sum32()%thumbShards]
}

// Get returns the cached entry if present and unexpired.
func (c *ThumbCache) Get(key string) (data []byte, contentType string, ok bool) {
	s := c.shardFor(key)
	s.mu.RLock()
	e := s.entries[key]
	s.mu.RUnlock()
	if e == nil {
		c.misses.Add(1)
		return nil, "", false
	}
	if time.Now().After(e.expiresAt) {
		s.mu.Lock()
		if cur := s.entries[key]; cur == e {
			delete(s.entries, key)
			s.bytes -= int64(len(cur.data))
			c.evicts.Add(1)
		}
		s.mu.Unlock()
		c.misses.Add(1)
		return nil, "", false
	}
	c.hits.Add(1)
	return e.data, e.contentType, true
}

// Put inserts (or replaces) an entry, evicting random entries from the
// shard if its byte budget is exceeded.
func (c *ThumbCache) Put(key string, data []byte, contentType string) {
	if len(data) == 0 || len(data) > thumbMaxItemSize {
		return
	}
	s := c.shardFor(key)
	e := &thumbEntry{
		data:        data,
		contentType: contentType,
		expiresAt:   time.Now().Add(c.ttl),
	}
	s.mu.Lock()
	if old, ok := s.entries[key]; ok {
		s.bytes -= int64(len(old.data))
	}
	s.entries[key] = e
	s.bytes += int64(len(data))
	c.stores.Add(1)

	for s.bytes > c.perShard {
		var victim string
		for k := range s.entries {
			if k == key {
				continue
			}
			victim = k
			break
		}
		if victim == "" {
			break
		}
		s.bytes -= int64(len(s.entries[victim].data))
		delete(s.entries, victim)
		c.evicts.Add(1)
	}
	s.mu.Unlock()
}

// getBuf returns a reset *bytes.Buffer from the pool.
func (c *ThumbCache) getBuf() *bytes.Buffer {
	b := c.bufPool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

// putBuf returns a buffer to the pool, discarding very large ones so the
// pool doesn't retain runaway allocations.
func (c *ThumbCache) putBuf(b *bytes.Buffer) {
	if b == nil || b.Cap() > thumbBufMaxKeep {
		return
	}
	c.bufPool.Put(b)
}

// Stats reports counters. Useful for /debug or logging.
func (c *ThumbCache) Stats() (hits, misses, stores, evicts int64) {
	return c.hits.Load(), c.misses.Load(), c.stores.Load(), c.evicts.Load()
}
