// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package idmclient

import (
	"sort"
	"time"

	"github.com/juju/utils/cache"
	"gopkg.in/errgo.v1"

	"github.com/juju/idmclient/params"
)

// GroupCache holds a cache of group membership information.
type GroupCache struct {
	cache  *cache.Cache
	client *Client
}

// NewGroupCache returns a GroupCache that will cache
// group membership information.
//
// It will cache results for at most cacheTime.
//
// Note that use of this type should be avoided when possible - in
// the future it may not be possible to enumerate group membership
// for a user.
func NewGroupCache(c *Client, cacheTime time.Duration) *GroupCache {
	return &GroupCache{
		cache:  cache.New(cacheTime),
		client: c,
	}
}

// Groups returns the set of groups that the user is a member of.
func (gc *GroupCache) Groups(username string) ([]string, error) {
	groupMap, err := gc.groupMap(username)
	if err != nil {
		return nil, errgo.Mask(err)
	}
	groups := make([]string, 0, len(groupMap))
	for g := range groupMap {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	return groups, nil
}

func (gc *GroupCache) groupMap(username string) (map[string]bool, error) {
	groups0, err := gc.cache.Get(username, func() (interface{}, error) {
		groups, err := gc.client.UserGroups(&params.UserGroupsRequest{
			Username: params.Username(username),
		})
		if err != nil && errgo.Cause(err) != params.ErrNotFound {
			return nil, errgo.Mask(err)
		}
		groupMap := make(map[string]bool)
		for _, g := range groups {
			groupMap[g] = true
		}
		return groupMap, nil
	})
	if err != nil {
		return nil, errgo.Notef(err, "cannot fetch groups")
	}
	return groups0.(map[string]bool), nil
}

// CacheEvict evicts username from the cache.
func (c *GroupCache) CacheEvict(username string) {
	c.cache.Evict(username)
}

// CacheEvictAll evicts everything from the cache.
func (c *GroupCache) CacheEvictAll() {
	c.cache.EvictAll()
}
