// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/yaml.v2"
)

// hashCache holds the current set of config for a particular entity.
// It stores hashes of config values for collections of config keys,
// so that hashing is done once per key combination instead of repeating the
// same operation for watchers watching the same set of keys.
type hashCache struct {
	config map[string]interface{}

	// The key to the hash map is the string keys of the watcher.
	// They must be sorted and comma delimited.
	hash map[string]string

	cacheHits   prometheus.Gauge
	cacheMisses prometheus.Gauge
	mu          sync.Mutex
}

func newHashCache(config map[string]interface{}, cacheHits, cacheMisses prometheus.Gauge) (*hashCache, string) {
	cache := &hashCache{
		config: config,
		hash:   make(map[string]string),

		cacheHits:   cacheHits,
		cacheMisses: cacheMisses,
	}

	// Generate the hash for the entire config.
	allHash := cache.generateHash(nil)
	cache.hash[""] = allHash
	return cache, allHash
}

func (c *hashCache) getHash(keys []string) string {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := strings.Join(keys, ",")
	value, found := c.hash[key]
	if found {
		c.incHits()
		return value
	}

	c.incMisses()
	value = c.generateHash(keys)
	c.hash[key] = value
	return value
}

func (c *hashCache) generateHash(keys []string) string {
	interested := c.config
	if len(keys) > 0 {
		interested = make(map[string]interface{})
		for _, key := range keys {
			if value, found := c.config[key]; found {
				interested[key] = value
			}
		}
	}
	h, err := hashSettings(interested)
	if err != nil {
		logger.Errorf("invariant error - model config should be yaml serializable and hashable, %v", err)
		return ""
	}
	return h
}

func (c *hashCache) incHits() {
	if c.cacheHits != nil {
		c.cacheHits.Inc()
	}
}

func (c *hashCache) incMisses() {
	if c.cacheMisses != nil {
		c.cacheMisses.Inc()
	}
}

// hash returns a hash of the string values.
func hash(values ...string) (string, error) {
	hash := sha256.New()
	for _, s := range values {
		_, err := hash.Write([]byte(s))
		if err != nil {
			return "", errors.Trace(err)
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// hashSettings returns a hash of the yaml serialized settings.
// If the settings are not able to be serialized an error is returned.
func hashSettings(settings map[string]interface{}, extras ...string) (string, error) {
	encoded, err := yaml.Marshal(settings)
	if err != nil {
		return "", errors.Trace(err)
	}
	values := append(extras, string(encoded))
	return hash(values...)
}
