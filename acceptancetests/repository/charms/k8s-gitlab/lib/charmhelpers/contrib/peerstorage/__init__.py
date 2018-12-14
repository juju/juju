# Copyright 2014-2015 Canonical Limited.
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

import json
import six

from charmhelpers.core.hookenv import relation_id as current_relation_id
from charmhelpers.core.hookenv import (
    is_relation_made,
    relation_ids,
    relation_get as _relation_get,
    local_unit,
    relation_set as _relation_set,
    leader_get as _leader_get,
    leader_set,
    is_leader,
)


"""
This helper provides functions to support use of a peer relation
for basic key/value storage, with the added benefit that all storage
can be replicated across peer units.

Requirement to use:

To use this, the "peer_echo()" method has to be called form the peer
relation's relation-changed hook:

@hooks.hook("cluster-relation-changed") # Adapt the to your peer relation name
def cluster_relation_changed():
    peer_echo()

Once this is done, you can use peer storage from anywhere:

@hooks.hook("some-hook")
def some_hook():
    # You can store and retrieve key/values this way:
    if is_relation_made("cluster"):  # from charmhelpers.core.hookenv
        # There are peers available so we can work with peer storage
        peer_store("mykey", "myvalue")
        value = peer_retrieve("mykey")
        print value
    else:
        print "No peers joind the relation, cannot share key/values :("
"""


def leader_get(attribute=None, rid=None):
    """Wrapper to ensure that settings are migrated from the peer relation.

    This is to support upgrading an environment that does not support
    Juju leadership election to one that does.

    If a setting is not extant in the leader-get but is on the relation-get
    peer rel, it is migrated and marked as such so that it is not re-migrated.
    """
    migration_key = '__leader_get_migrated_settings__'
    if not is_leader():
        return _leader_get(attribute=attribute)

    settings_migrated = False
    leader_settings = _leader_get(attribute=attribute)
    previously_migrated = _leader_get(attribute=migration_key)

    if previously_migrated:
        migrated = set(json.loads(previously_migrated))
    else:
        migrated = set([])

    try:
        if migration_key in leader_settings:
            del leader_settings[migration_key]
    except TypeError:
        pass

    if attribute:
        if attribute in migrated:
            return leader_settings

        # If attribute not present in leader db, check if this unit has set
        # the attribute in the peer relation
        if not leader_settings:
            peer_setting = _relation_get(attribute=attribute, unit=local_unit(),
                                         rid=rid)
            if peer_setting:
                leader_set(settings={attribute: peer_setting})
                leader_settings = peer_setting

        if leader_settings:
            settings_migrated = True
            migrated.add(attribute)
    else:
        r_settings = _relation_get(unit=local_unit(), rid=rid)
        if r_settings:
            for key in set(r_settings.keys()).difference(migrated):
                # Leader setting wins
                if not leader_settings.get(key):
                    leader_settings[key] = r_settings[key]

                settings_migrated = True
                migrated.add(key)

            if settings_migrated:
                leader_set(**leader_settings)

    if migrated and settings_migrated:
        migrated = json.dumps(list(migrated))
        leader_set(settings={migration_key: migrated})

    return leader_settings


def relation_set(relation_id=None, relation_settings=None, **kwargs):
    """Attempt to use leader-set if supported in the current version of Juju,
    otherwise falls back on relation-set.

    Note that we only attempt to use leader-set if the provided relation_id is
    a peer relation id or no relation id is provided (in which case we assume
    we are within the peer relation context).
    """
    try:
        if relation_id in relation_ids('cluster'):
            return leader_set(settings=relation_settings, **kwargs)
        else:
            raise NotImplementedError
    except NotImplementedError:
        return _relation_set(relation_id=relation_id,
                             relation_settings=relation_settings, **kwargs)


def relation_get(attribute=None, unit=None, rid=None):
    """Attempt to use leader-get if supported in the current version of Juju,
    otherwise falls back on relation-get.

    Note that we only attempt to use leader-get if the provided rid is a peer
    relation id or no relation id is provided (in which case we assume we are
    within the peer relation context).
    """
    try:
        if rid in relation_ids('cluster'):
            return leader_get(attribute, rid)
        else:
            raise NotImplementedError
    except NotImplementedError:
        return _relation_get(attribute=attribute, rid=rid, unit=unit)


def peer_retrieve(key, relation_name='cluster'):
    """Retrieve a named key from peer relation `relation_name`."""
    cluster_rels = relation_ids(relation_name)
    if len(cluster_rels) > 0:
        cluster_rid = cluster_rels[0]
        return relation_get(attribute=key, rid=cluster_rid,
                            unit=local_unit())
    else:
        raise ValueError('Unable to detect'
                         'peer relation {}'.format(relation_name))


def peer_retrieve_by_prefix(prefix, relation_name='cluster', delimiter='_',
                            inc_list=None, exc_list=None):
    """ Retrieve k/v pairs given a prefix and filter using {inc,exc}_list """
    inc_list = inc_list if inc_list else []
    exc_list = exc_list if exc_list else []
    peerdb_settings = peer_retrieve('-', relation_name=relation_name)
    matched = {}
    if peerdb_settings is None:
        return matched
    for k, v in peerdb_settings.items():
        full_prefix = prefix + delimiter
        if k.startswith(full_prefix):
            new_key = k.replace(full_prefix, '')
            if new_key in exc_list:
                continue
            if new_key in inc_list or len(inc_list) == 0:
                matched[new_key] = v
    return matched


def peer_store(key, value, relation_name='cluster'):
    """Store the key/value pair on the named peer relation `relation_name`."""
    cluster_rels = relation_ids(relation_name)
    if len(cluster_rels) > 0:
        cluster_rid = cluster_rels[0]
        relation_set(relation_id=cluster_rid,
                     relation_settings={key: value})
    else:
        raise ValueError('Unable to detect '
                         'peer relation {}'.format(relation_name))


def peer_echo(includes=None, force=False):
    """Echo filtered attributes back onto the same relation for storage.

    This is a requirement to use the peerstorage module - it needs to be called
    from the peer relation's changed hook.

    If Juju leader support exists this will be a noop unless force is True.
    """
    try:
        is_leader()
    except NotImplementedError:
        pass
    else:
        if not force:
            return  # NOOP if leader-election is supported

    # Use original non-leader calls
    relation_get = _relation_get
    relation_set = _relation_set

    rdata = relation_get()
    echo_data = {}
    if includes is None:
        echo_data = rdata.copy()
        for ex in ['private-address', 'public-address']:
            if ex in echo_data:
                echo_data.pop(ex)
    else:
        for attribute, value in six.iteritems(rdata):
            for include in includes:
                if include in attribute:
                    echo_data[attribute] = value
    if len(echo_data) > 0:
        relation_set(relation_settings=echo_data)


def peer_store_and_set(relation_id=None, peer_relation_name='cluster',
                       peer_store_fatal=False, relation_settings=None,
                       delimiter='_', **kwargs):
    """Store passed-in arguments both in argument relation and in peer storage.

    It functions like doing relation_set() and peer_store() at the same time,
    with the same data.

    @param relation_id: the id of the relation to store the data on. Defaults
                        to the current relation.
    @param peer_store_fatal: Set to True, the function will raise an exception
                             should the peer sotrage not be avialable."""

    relation_settings = relation_settings if relation_settings else {}
    relation_set(relation_id=relation_id,
                 relation_settings=relation_settings,
                 **kwargs)
    if is_relation_made(peer_relation_name):
        for key, value in six.iteritems(dict(list(kwargs.items()) +
                                             list(relation_settings.items()))):
            key_prefix = relation_id or current_relation_id()
            peer_store(key_prefix + delimiter + key,
                       value,
                       relation_name=peer_relation_name)
    else:
        if peer_store_fatal:
            raise ValueError('Unable to detect '
                             'peer relation {}'.format(peer_relation_name))
