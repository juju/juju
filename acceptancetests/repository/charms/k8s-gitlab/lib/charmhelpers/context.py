# Copyright 2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

'''
A Pythonic API to interact with the charm hook environment.

:author: Stuart Bishop <stuart.bishop@canonical.com>
'''

import six

from charmhelpers.core import hookenv

from collections import OrderedDict
if six.PY3:
    from collections import UserDict  # pragma: nocover
else:
    from UserDict import IterableUserDict as UserDict  # pragma: nocover


class Relations(OrderedDict):
    '''Mapping relation name -> relation id -> Relation.

    >>> rels = Relations()
    >>> rels['sprog']['sprog:12']['client/6']['widget']
    'remote widget'
    >>> rels['sprog']['sprog:12'].local['widget'] = 'local widget'
    >>> rels['sprog']['sprog:12'].local['widget']
    'local widget'
    >>> rels.peer.local['widget']
    'local widget on the peer relation'
    '''
    def __init__(self):
        super(Relations, self).__init__()
        for relname in sorted(hookenv.relation_types()):
            self[relname] = OrderedDict()
            relids = hookenv.relation_ids(relname)
            relids.sort(key=lambda x: int(x.split(':', 1)[-1]))
            for relid in relids:
                self[relname][relid] = Relation(relid)

    @property
    def peer(self):
        peer_relid = hookenv.peer_relation_id()
        for rels in self.values():
            if peer_relid in rels:
                return rels[peer_relid]


class Relation(OrderedDict):
    '''Mapping of unit -> remote RelationInfo for a relation.

    This is an OrderedDict mapping, ordered numerically by
    by unit number.

    Also provides access to the local RelationInfo, and peer RelationInfo
    instances by the 'local' and 'peers' attributes.

    >>> r = Relation('sprog:12')
    >>> r.keys()
    ['client/9', 'client/10']     # Ordered numerically
    >>> r['client/10']['widget']  # A remote RelationInfo setting
    'remote widget'
    >>> r.local['widget']         # The local RelationInfo setting
    'local widget'
    '''
    relid = None    # The relation id.
    relname = None  # The relation name (also known as relation type).
    service = None  # The remote service name, if known.
    local = None    # The local end's RelationInfo.
    peers = None    # Map of peer -> RelationInfo. None if no peer relation.

    def __init__(self, relid):
        remote_units = hookenv.related_units(relid)
        remote_units.sort(key=lambda u: int(u.split('/', 1)[-1]))
        super(Relation, self).__init__((unit, RelationInfo(relid, unit))
                                       for unit in remote_units)

        self.relname = relid.split(':', 1)[0]
        self.relid = relid
        self.local = RelationInfo(relid, hookenv.local_unit())

        for relinfo in self.values():
            self.service = relinfo.service
            break

        # If we have peers, and they have joined both the provided peer
        # relation and this relation, we can peek at their data too.
        # This is useful for creating consensus without leadership.
        peer_relid = hookenv.peer_relation_id()
        if peer_relid and peer_relid != relid:
            peers = hookenv.related_units(peer_relid)
            if peers:
                peers.sort(key=lambda u: int(u.split('/', 1)[-1]))
                self.peers = OrderedDict((peer, RelationInfo(relid, peer))
                                         for peer in peers)
            else:
                self.peers = OrderedDict()
        else:
            self.peers = None

    def __str__(self):
        return '{} ({})'.format(self.relid, self.service)


class RelationInfo(UserDict):
    '''The bag of data at an end of a relation.

    Every unit participating in a relation has a single bag of
    data associated with that relation. This is that bag.

    The bag of data for the local unit may be updated. Remote data
    is immutable and will remain static for the duration of the hook.

    Changes made to the local units relation data only become visible
    to other units after the hook completes successfully. If the hook
    does not complete successfully, the changes are rolled back.

    Unlike standard Python mappings, setting an item to None is the
    same as deleting it.

    >>> relinfo = RelationInfo('db:12')  # Default is the local unit.
    >>> relinfo['user'] = 'fred'
    >>> relinfo['user']
    'fred'
    >>> relinfo['user'] = None
    >>> 'fred' in relinfo
    False

    This class wraps hookenv.relation_get and hookenv.relation_set.
    All caching is left up to these two methods to avoid synchronization
    issues. Data is only loaded on demand.
    '''
    relid = None    # The relation id.
    relname = None  # The relation name (also know as the relation type).
    unit = None     # The unit id.
    number = None   # The unit number (integer).
    service = None  # The service name.

    def __init__(self, relid, unit):
        self.relname = relid.split(':', 1)[0]
        self.relid = relid
        self.unit = unit
        self.service, num = self.unit.split('/', 1)
        self.number = int(num)

    def __str__(self):
        return '{} ({})'.format(self.relid, self.unit)

    @property
    def data(self):
        return hookenv.relation_get(rid=self.relid, unit=self.unit)

    def __setitem__(self, key, value):
        if self.unit != hookenv.local_unit():
            raise TypeError('Attempting to set {} on remote unit {}'
                            ''.format(key, self.unit))
        if value is not None and not isinstance(value, six.string_types):
            # We don't do implicit casting. This would cause simple
            # types like integers to be read back as strings in subsequent
            # hooks, and mutable types would require a lot of wrapping
            # to ensure relation-set gets called when they are mutated.
            raise ValueError('Only string values allowed')
        hookenv.relation_set(self.relid, {key: value})

    def __delitem__(self, key):
        # Deleting a key and setting it to null is the same thing in
        # Juju relations.
        self[key] = None


class Leader(UserDict):
    def __init__(self):
        pass  # Don't call superclass initializer, as it will nuke self.data

    @property
    def data(self):
        return hookenv.leader_get()

    def __setitem__(self, key, value):
        if not hookenv.is_leader():
            raise TypeError('Not the leader. Cannot change leader settings.')
        if value is not None and not isinstance(value, six.string_types):
            # We don't do implicit casting. This would cause simple
            # types like integers to be read back as strings in subsequent
            # hooks, and mutable types would require a lot of wrapping
            # to ensure leader-set gets called when they are mutated.
            raise ValueError('Only string values allowed')
        hookenv.leader_set({key: value})

    def __delitem__(self, key):
        # Deleting a key and setting it to null is the same thing in
        # Juju leadership settings.
        self[key] = None
