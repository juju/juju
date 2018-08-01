// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package idmclient

import (
	"time"

	"gopkg.in/errgo.v1"
)

// TODO unexport this type - it's best exposed as part of the client API only.

// PermChecker provides a way to query ACLs using the identity client.
type PermChecker struct {
	cache *GroupCache
}

// NewPermChecker returns a permission checker
// that uses the given identity client to check permissions.
//
// It will cache results for at most cacheTime.
func NewPermChecker(c *Client, cacheTime time.Duration) *PermChecker {
	return &PermChecker{
		cache: NewGroupCache(c, cacheTime),
	}
}

// NewPermCheckerWithCache returns a new PermChecker using
// the given cache for its group queries.
func NewPermCheckerWithCache(cache *GroupCache) *PermChecker {
	return &PermChecker{
		cache: cache,
	}
}

// Allow reports whether the given ACL admits the user with the given
// name. If the user does not exist and the ACL does not allow username
// or everyone, it will return (false, nil).
func (c *PermChecker) Allow(username string, acl []string) (bool, error) {
	if len(acl) == 0 {
		return false, nil
	}
	for _, name := range acl {
		if name == "everyone" || name == username {
			return true, nil
		}
	}
	groups, err := c.cache.groupMap(username)
	if err != nil {
		return false, errgo.Mask(err)
	}
	for _, a := range acl {
		if groups[a] {
			return true, nil
		}
	}
	return false, nil
}

// CacheEvict evicts username from the cache.
func (c *PermChecker) CacheEvict(username string) {
	c.cache.CacheEvict(username)
}

// CacheEvictAll evicts everything from the cache.
func (c *PermChecker) CacheEvictAll() {
	c.cache.CacheEvictAll()
}
