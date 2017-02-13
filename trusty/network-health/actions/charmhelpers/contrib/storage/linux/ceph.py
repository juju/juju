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

#
# Copyright 2012 Canonical Ltd.
#
# This file is sourced from lp:openstack-charm-helpers
#
# Authors:
#  James Page <james.page@ubuntu.com>
#  Adam Gandelman <adamg@ubuntu.com>
#

import errno
import hashlib
import math
import six

import os
import shutil
import json
import time
import uuid

from subprocess import (
    check_call,
    check_output,
    CalledProcessError,
)
from charmhelpers.core.hookenv import (
    config,
    local_unit,
    relation_get,
    relation_ids,
    relation_set,
    related_units,
    log,
    DEBUG,
    INFO,
    WARNING,
    ERROR,
)
from charmhelpers.core.host import (
    mount,
    mounts,
    service_start,
    service_stop,
    service_running,
    umount,
)
from charmhelpers.fetch import (
    apt_install,
)

from charmhelpers.core.kernel import modprobe
from charmhelpers.contrib.openstack.utils import config_flags_parser

KEYRING = '/etc/ceph/ceph.client.{}.keyring'
KEYFILE = '/etc/ceph/ceph.client.{}.key'

CEPH_CONF = """[global]
auth supported = {auth}
keyring = {keyring}
mon host = {mon_hosts}
log to syslog = {use_syslog}
err to syslog = {use_syslog}
clog to syslog = {use_syslog}
"""

# The number of placement groups per OSD to target for placement group
# calculations. This number is chosen as 100 due to the ceph PG Calc
# documentation recommending to choose 100 for clusters which are not
# expected to increase in the foreseeable future. Since the majority of the
# calculations are done on deployment, target the case of non-expanding
# clusters as the default.
DEFAULT_PGS_PER_OSD_TARGET = 100
DEFAULT_POOL_WEIGHT = 10.0
LEGACY_PG_COUNT = 200
DEFAULT_MINIMUM_PGS = 2


def validator(value, valid_type, valid_range=None):
    """
    Used to validate these: http://docs.ceph.com/docs/master/rados/operations/pools/#set-pool-values
    Example input:
        validator(value=1,
                  valid_type=int,
                  valid_range=[0, 2])
    This says I'm testing value=1.  It must be an int inclusive in [0,2]

    :param value: The value to validate
    :param valid_type: The type that value should be.
    :param valid_range: A range of values that value can assume.
    :return:
    """
    assert isinstance(value, valid_type), "{} is not a {}".format(
        value,
        valid_type)
    if valid_range is not None:
        assert isinstance(valid_range, list), \
            "valid_range must be a list, was given {}".format(valid_range)
        # If we're dealing with strings
        if valid_type is six.string_types:
            assert value in valid_range, \
                "{} is not in the list {}".format(value, valid_range)
        # Integer, float should have a min and max
        else:
            if len(valid_range) != 2:
                raise ValueError(
                    "Invalid valid_range list of {} for {}.  "
                    "List must be [min,max]".format(valid_range, value))
            assert value >= valid_range[0], \
                "{} is less than minimum allowed value of {}".format(
                    value, valid_range[0])
            assert value <= valid_range[1], \
                "{} is greater than maximum allowed value of {}".format(
                    value, valid_range[1])


class PoolCreationError(Exception):
    """
    A custom error to inform the caller that a pool creation failed.  Provides an error message
    """

    def __init__(self, message):
        super(PoolCreationError, self).__init__(message)


class Pool(object):
    """
    An object oriented approach to Ceph pool creation. This base class is inherited by ReplicatedPool and ErasurePool.
    Do not call create() on this base class as it will not do anything.  Instantiate a child class and call create().
    """

    def __init__(self, service, name):
        self.service = service
        self.name = name

    # Create the pool if it doesn't exist already
    # To be implemented by subclasses
    def create(self):
        pass

    def add_cache_tier(self, cache_pool, mode):
        """
        Adds a new cache tier to an existing pool.
        :param cache_pool: six.string_types.  The cache tier pool name to add.
        :param mode: six.string_types. The caching mode to use for this pool.  valid range = ["readonly", "writeback"]
        :return: None
        """
        # Check the input types and values
        validator(value=cache_pool, valid_type=six.string_types)
        validator(value=mode, valid_type=six.string_types, valid_range=["readonly", "writeback"])

        check_call(['ceph', '--id', self.service, 'osd', 'tier', 'add', self.name, cache_pool])
        check_call(['ceph', '--id', self.service, 'osd', 'tier', 'cache-mode', cache_pool, mode])
        check_call(['ceph', '--id', self.service, 'osd', 'tier', 'set-overlay', self.name, cache_pool])
        check_call(['ceph', '--id', self.service, 'osd', 'pool', 'set', cache_pool, 'hit_set_type', 'bloom'])

    def remove_cache_tier(self, cache_pool):
        """
        Removes a cache tier from Ceph.  Flushes all dirty objects from writeback pools and waits for that to complete.
        :param cache_pool: six.string_types.  The cache tier pool name to remove.
        :return: None
        """
        # read-only is easy, writeback is much harder
        mode = get_cache_mode(self.service, cache_pool)
        version = ceph_version()
        if mode == 'readonly':
            check_call(['ceph', '--id', self.service, 'osd', 'tier', 'cache-mode', cache_pool, 'none'])
            check_call(['ceph', '--id', self.service, 'osd', 'tier', 'remove', self.name, cache_pool])

        elif mode == 'writeback':
            pool_forward_cmd = ['ceph', '--id', self.service, 'osd', 'tier',
                                'cache-mode', cache_pool, 'forward']
            if version >= '10.1':
                # Jewel added a mandatory flag
                pool_forward_cmd.append('--yes-i-really-mean-it')

            check_call(pool_forward_cmd)
            # Flush the cache and wait for it to return
            check_call(['rados', '--id', self.service, '-p', cache_pool, 'cache-flush-evict-all'])
            check_call(['ceph', '--id', self.service, 'osd', 'tier', 'remove-overlay', self.name])
            check_call(['ceph', '--id', self.service, 'osd', 'tier', 'remove', self.name, cache_pool])

    def get_pgs(self, pool_size, percent_data=DEFAULT_POOL_WEIGHT):
        """Return the number of placement groups to use when creating the pool.

        Returns the number of placement groups which should be specified when
        creating the pool. This is based upon the calculation guidelines
        provided by the Ceph Placement Group Calculator (located online at
        http://ceph.com/pgcalc/).

        The number of placement groups are calculated using the following:

            (Target PGs per OSD) * (OSD #) * (%Data)
            ----------------------------------------
                         (Pool size)

        Per the upstream guidelines, the OSD # should really be considered
        based on the number of OSDs which are eligible to be selected by the
        pool. Since the pool creation doesn't specify any of CRUSH set rules,
        the default rule will be dependent upon the type of pool being
        created (replicated or erasure).

        This code makes no attempt to determine the number of OSDs which can be
        selected for the specific rule, rather it is left to the user to tune
        in the form of 'expected-osd-count' config option.

        :param pool_size: int. pool_size is either the number of replicas for
            replicated pools or the K+M sum for erasure coded pools
        :param percent_data: float. the percentage of data that is expected to
            be contained in the pool for the specific OSD set. Default value
            is to assume 10% of the data is for this pool, which is a
            relatively low % of the data but allows for the pg_num to be
            increased. NOTE: the default is primarily to handle the scenario
            where related charms requiring pools has not been upgraded to
            include an update to indicate their relative usage of the pools.
        :return: int.  The number of pgs to use.
        """

        # Note: This calculation follows the approach that is provided
        # by the Ceph PG Calculator located at http://ceph.com/pgcalc/.
        validator(value=pool_size, valid_type=int)

        # Ensure that percent data is set to something - even with a default
        # it can be set to None, which would wreak havoc below.
        if percent_data is None:
            percent_data = DEFAULT_POOL_WEIGHT

        # If the expected-osd-count is specified, then use the max between
        # the expected-osd-count and the actual osd_count
        osd_list = get_osds(self.service)
        expected = config('expected-osd-count') or 0

        if osd_list:
            osd_count = max(expected, len(osd_list))

            # Log a message to provide some insight if the calculations claim
            # to be off because someone is setting the expected count and
            # there are more OSDs in reality. Try to make a proper guess
            # based upon the cluster itself.
            if expected and osd_count != expected:
                log("Found more OSDs than provided expected count. "
                    "Using the actual count instead", INFO)
        elif expected:
            # Use the expected-osd-count in older ceph versions to allow for
            # a more accurate pg calculations
            osd_count = expected
        else:
            # NOTE(james-page): Default to 200 for older ceph versions
            # which don't support OSD query from cli
            return LEGACY_PG_COUNT

        percent_data /= 100.0
        target_pgs_per_osd = config('pgs-per-osd') or DEFAULT_PGS_PER_OSD_TARGET
        num_pg = (target_pgs_per_osd * osd_count * percent_data) // pool_size

        # NOTE: ensure a sane minimum number of PGS otherwise we don't get any
        #       reasonable data distribution in minimal OSD configurations
        if num_pg < DEFAULT_MINIMUM_PGS:
            num_pg = DEFAULT_MINIMUM_PGS

        # The CRUSH algorithm has a slight optimization for placement groups
        # with powers of 2 so find the nearest power of 2. If the nearest
        # power of 2 is more than 25% below the original value, the next
        # highest value is used. To do this, find the nearest power of 2 such
        # that 2^n <= num_pg, check to see if its within the 25% tolerance.
        exponent = math.floor(math.log(num_pg, 2))
        nearest = 2 ** exponent
        if (num_pg - nearest) > (num_pg * 0.25):
            # Choose the next highest power of 2 since the nearest is more
            # than 25% below the original value.
            return int(nearest * 2)
        else:
            return int(nearest)


class ReplicatedPool(Pool):
    def __init__(self, service, name, pg_num=None, replicas=2,
                 percent_data=10.0):
        super(ReplicatedPool, self).__init__(service=service, name=name)
        self.replicas = replicas
        if pg_num:
            # Since the number of placement groups were specified, ensure
            # that there aren't too many created.
            max_pgs = self.get_pgs(self.replicas, 100.0)
            self.pg_num = min(pg_num, max_pgs)
        else:
            self.pg_num = self.get_pgs(self.replicas, percent_data)

    def create(self):
        if not pool_exists(self.service, self.name):
            # Create it
            cmd = ['ceph', '--id', self.service, 'osd', 'pool', 'create',
                   self.name, str(self.pg_num)]
            try:
                check_call(cmd)
                # Set the pool replica size
                update_pool(client=self.service,
                            pool=self.name,
                            settings={'size': str(self.replicas)})
            except CalledProcessError:
                raise


# Default jerasure erasure coded pool
class ErasurePool(Pool):
    def __init__(self, service, name, erasure_code_profile="default",
                 percent_data=10.0):
        super(ErasurePool, self).__init__(service=service, name=name)
        self.erasure_code_profile = erasure_code_profile
        self.percent_data = percent_data

    def create(self):
        if not pool_exists(self.service, self.name):
            # Try to find the erasure profile information in order to properly
            # size the number of placement groups. The size of an erasure
            # coded placement group is calculated as k+m.
            erasure_profile = get_erasure_profile(self.service,
                                                  self.erasure_code_profile)

            # Check for errors
            if erasure_profile is None:
                msg = ("Failed to discover erasure profile named "
                       "{}".format(self.erasure_code_profile))
                log(msg, level=ERROR)
                raise PoolCreationError(msg)
            if 'k' not in erasure_profile or 'm' not in erasure_profile:
                # Error
                msg = ("Unable to find k (data chunks) or m (coding chunks) "
                       "in erasure profile {}".format(erasure_profile))
                log(msg, level=ERROR)
                raise PoolCreationError(msg)

            k = int(erasure_profile['k'])
            m = int(erasure_profile['m'])
            pgs = self.get_pgs(k + m, self.percent_data)
            # Create it
            cmd = ['ceph', '--id', self.service, 'osd', 'pool', 'create',
                   self.name, str(pgs), str(pgs),
                   'erasure', self.erasure_code_profile]
            try:
                check_call(cmd)
            except CalledProcessError:
                raise

    """Get an existing erasure code profile if it already exists.
       Returns json formatted output"""


def get_mon_map(service):
    """
    Returns the current monitor map.
    :param service: six.string_types. The Ceph user name to run the command under
    :return: json string. :raise: ValueError if the monmap fails to parse.
      Also raises CalledProcessError if our ceph command fails
    """
    try:
        mon_status = check_output(
            ['ceph', '--id', service,
             'mon_status', '--format=json'])
        try:
            return json.loads(mon_status)
        except ValueError as v:
            log("Unable to parse mon_status json: {}. Error: {}".format(
                mon_status, v.message))
            raise
    except CalledProcessError as e:
        log("mon_status command failed with message: {}".format(
            e.message))
        raise


def hash_monitor_names(service):
    """
    Uses the get_mon_map() function to get information about the monitor
    cluster.
    Hash the name of each monitor.  Return a sorted list of monitor hashes
    in an ascending order.
    :param service: six.string_types. The Ceph user name to run the command under
    :rtype : dict.   json dict of monitor name, ip address and rank
    example: {
        'name': 'ip-172-31-13-165',
        'rank': 0,
        'addr': '172.31.13.165:6789/0'}
    """
    try:
        hash_list = []
        monitor_list = get_mon_map(service=service)
        if monitor_list['monmap']['mons']:
            for mon in monitor_list['monmap']['mons']:
                hash_list.append(
                    hashlib.sha224(mon['name'].encode('utf-8')).hexdigest())
            return sorted(hash_list)
        else:
            return None
    except (ValueError, CalledProcessError):
        raise


def monitor_key_delete(service, key):
    """
    Delete a key and value pair from the monitor cluster
    :param service: six.string_types. The Ceph user name to run the command under
    Deletes a key value pair on the monitor cluster.
    :param key: six.string_types.  The key to delete.
    """
    try:
        check_output(
            ['ceph', '--id', service,
             'config-key', 'del', str(key)])
    except CalledProcessError as e:
        log("Monitor config-key put failed with message: {}".format(
            e.output))
        raise


def monitor_key_set(service, key, value):
    """
    Sets a key value pair on the monitor cluster.
    :param service: six.string_types. The Ceph user name to run the command under
    :param key: six.string_types.  The key to set.
    :param value: The value to set.  This will be converted to a string
        before setting
    """
    try:
        check_output(
            ['ceph', '--id', service,
             'config-key', 'put', str(key), str(value)])
    except CalledProcessError as e:
        log("Monitor config-key put failed with message: {}".format(
            e.output))
        raise


def monitor_key_get(service, key):
    """
    Gets the value of an existing key in the monitor cluster.
    :param service: six.string_types. The Ceph user name to run the command under
    :param key: six.string_types.  The key to search for.
    :return: Returns the value of that key or None if not found.
    """
    try:
        output = check_output(
            ['ceph', '--id', service,
             'config-key', 'get', str(key)])
        return output
    except CalledProcessError as e:
        log("Monitor config-key get failed with message: {}".format(
            e.output))
        return None


def monitor_key_exists(service, key):
    """
    Searches for the existence of a key in the monitor cluster.
    :param service: six.string_types. The Ceph user name to run the command under
    :param key: six.string_types.  The key to search for
    :return: Returns True if the key exists, False if not and raises an
     exception if an unknown error occurs. :raise: CalledProcessError if
     an unknown error occurs
    """
    try:
        check_call(
            ['ceph', '--id', service,
             'config-key', 'exists', str(key)])
        # I can return true here regardless because Ceph returns
        # ENOENT if the key wasn't found
        return True
    except CalledProcessError as e:
        if e.returncode == errno.ENOENT:
            return False
        else:
            log("Unknown error from ceph config-get exists: {} {}".format(
                e.returncode, e.output))
            raise


def get_erasure_profile(service, name):
    """
    :param service: six.string_types. The Ceph user name to run the command under
    :param name:
    :return:
    """
    try:
        out = check_output(['ceph', '--id', service,
                            'osd', 'erasure-code-profile', 'get',
                            name, '--format=json'])
        return json.loads(out)
    except (CalledProcessError, OSError, ValueError):
        return None


def pool_set(service, pool_name, key, value):
    """
    Sets a value for a RADOS pool in ceph.
    :param service: six.string_types. The Ceph user name to run the command under
    :param pool_name: six.string_types
    :param key: six.string_types
    :param value:
    :return: None.  Can raise CalledProcessError
    """
    cmd = ['ceph', '--id', service, 'osd', 'pool', 'set', pool_name, key, value]
    try:
        check_call(cmd)
    except CalledProcessError:
        raise


def snapshot_pool(service, pool_name, snapshot_name):
    """
    Snapshots a RADOS pool in ceph.
    :param service: six.string_types. The Ceph user name to run the command under
    :param pool_name: six.string_types
    :param snapshot_name: six.string_types
    :return: None.  Can raise CalledProcessError
    """
    cmd = ['ceph', '--id', service, 'osd', 'pool', 'mksnap', pool_name, snapshot_name]
    try:
        check_call(cmd)
    except CalledProcessError:
        raise


def remove_pool_snapshot(service, pool_name, snapshot_name):
    """
    Remove a snapshot from a RADOS pool in ceph.
    :param service: six.string_types. The Ceph user name to run the command under
    :param pool_name: six.string_types
    :param snapshot_name: six.string_types
    :return: None.  Can raise CalledProcessError
    """
    cmd = ['ceph', '--id', service, 'osd', 'pool', 'rmsnap', pool_name, snapshot_name]
    try:
        check_call(cmd)
    except CalledProcessError:
        raise


# max_bytes should be an int or long
def set_pool_quota(service, pool_name, max_bytes):
    """
    :param service: six.string_types. The Ceph user name to run the command under
    :param pool_name: six.string_types
    :param max_bytes: int or long
    :return: None.  Can raise CalledProcessError
    """
    # Set a byte quota on a RADOS pool in ceph.
    cmd = ['ceph', '--id', service, 'osd', 'pool', 'set-quota', pool_name,
           'max_bytes', str(max_bytes)]
    try:
        check_call(cmd)
    except CalledProcessError:
        raise


def remove_pool_quota(service, pool_name):
    """
    Set a byte quota on a RADOS pool in ceph.
    :param service: six.string_types. The Ceph user name to run the command under
    :param pool_name: six.string_types
    :return: None.  Can raise CalledProcessError
    """
    cmd = ['ceph', '--id', service, 'osd', 'pool', 'set-quota', pool_name, 'max_bytes', '0']
    try:
        check_call(cmd)
    except CalledProcessError:
        raise


def remove_erasure_profile(service, profile_name):
    """
    Create a new erasure code profile if one does not already exist for it.  Updates
    the profile if it exists. Please see http://docs.ceph.com/docs/master/rados/operations/erasure-code-profile/
    for more details
    :param service: six.string_types. The Ceph user name to run the command under
    :param profile_name: six.string_types
    :return: None.  Can raise CalledProcessError
    """
    cmd = ['ceph', '--id', service, 'osd', 'erasure-code-profile', 'rm',
           profile_name]
    try:
        check_call(cmd)
    except CalledProcessError:
        raise


def create_erasure_profile(service, profile_name, erasure_plugin_name='jerasure',
                           failure_domain='host',
                           data_chunks=2, coding_chunks=1,
                           locality=None, durability_estimator=None):
    """
    Create a new erasure code profile if one does not already exist for it.  Updates
    the profile if it exists. Please see http://docs.ceph.com/docs/master/rados/operations/erasure-code-profile/
    for more details
    :param service: six.string_types. The Ceph user name to run the command under
    :param profile_name: six.string_types
    :param erasure_plugin_name: six.string_types
    :param failure_domain: six.string_types.  One of ['chassis', 'datacenter', 'host', 'osd', 'pdu', 'pod', 'rack', 'region',
        'room', 'root', 'row'])
    :param data_chunks: int
    :param coding_chunks: int
    :param locality: int
    :param durability_estimator: int
    :return: None.  Can raise CalledProcessError
    """
    # Ensure this failure_domain is allowed by Ceph
    validator(failure_domain, six.string_types,
              ['chassis', 'datacenter', 'host', 'osd', 'pdu', 'pod', 'rack', 'region', 'room', 'root', 'row'])

    cmd = ['ceph', '--id', service, 'osd', 'erasure-code-profile', 'set', profile_name,
           'plugin=' + erasure_plugin_name, 'k=' + str(data_chunks), 'm=' + str(coding_chunks),
           'ruleset_failure_domain=' + failure_domain]
    if locality is not None and durability_estimator is not None:
        raise ValueError("create_erasure_profile should be called with k, m and one of l or c but not both.")

    # Add plugin specific information
    if locality is not None:
        # For local erasure codes
        cmd.append('l=' + str(locality))
    if durability_estimator is not None:
        # For Shec erasure codes
        cmd.append('c=' + str(durability_estimator))

    if erasure_profile_exists(service, profile_name):
        cmd.append('--force')

    try:
        check_call(cmd)
    except CalledProcessError:
        raise


def rename_pool(service, old_name, new_name):
    """
    Rename a Ceph pool from old_name to new_name
    :param service: six.string_types. The Ceph user name to run the command under
    :param old_name: six.string_types
    :param new_name: six.string_types
    :return: None
    """
    validator(value=old_name, valid_type=six.string_types)
    validator(value=new_name, valid_type=six.string_types)

    cmd = ['ceph', '--id', service, 'osd', 'pool', 'rename', old_name, new_name]
    check_call(cmd)


def erasure_profile_exists(service, name):
    """
    Check to see if an Erasure code profile already exists.
    :param service: six.string_types. The Ceph user name to run the command under
    :param name: six.string_types
    :return: int or None
    """
    validator(value=name, valid_type=six.string_types)
    try:
        check_call(['ceph', '--id', service,
                    'osd', 'erasure-code-profile', 'get',
                    name])
        return True
    except CalledProcessError:
        return False


def get_cache_mode(service, pool_name):
    """
    Find the current caching mode of the pool_name given.
    :param service: six.string_types. The Ceph user name to run the command under
    :param pool_name: six.string_types
    :return: int or None
    """
    validator(value=service, valid_type=six.string_types)
    validator(value=pool_name, valid_type=six.string_types)
    out = check_output(['ceph', '--id', service, 'osd', 'dump', '--format=json'])
    try:
        osd_json = json.loads(out)
        for pool in osd_json['pools']:
            if pool['pool_name'] == pool_name:
                return pool['cache_mode']
        return None
    except ValueError:
        raise


def pool_exists(service, name):
    """Check to see if a RADOS pool already exists."""
    try:
        out = check_output(['rados', '--id', service,
                            'lspools']).decode('UTF-8')
    except CalledProcessError:
        return False

    return name in out.split()


def get_osds(service):
    """Return a list of all Ceph Object Storage Daemons currently in the
    cluster.
    """
    version = ceph_version()
    if version and version >= '0.56':
        return json.loads(check_output(['ceph', '--id', service,
                                        'osd', 'ls',
                                        '--format=json']).decode('UTF-8'))

    return None


def install():
    """Basic Ceph client installation."""
    ceph_dir = "/etc/ceph"
    if not os.path.exists(ceph_dir):
        os.mkdir(ceph_dir)

    apt_install('ceph-common', fatal=True)


def rbd_exists(service, pool, rbd_img):
    """Check to see if a RADOS block device exists."""
    try:
        out = check_output(['rbd', 'list', '--id',
                            service, '--pool', pool]).decode('UTF-8')
    except CalledProcessError:
        return False

    return rbd_img in out


def create_rbd_image(service, pool, image, sizemb):
    """Create a new RADOS block device."""
    cmd = ['rbd', 'create', image, '--size', str(sizemb), '--id', service,
           '--pool', pool]
    check_call(cmd)


def update_pool(client, pool, settings):
    cmd = ['ceph', '--id', client, 'osd', 'pool', 'set', pool]
    for k, v in six.iteritems(settings):
        cmd.append(k)
        cmd.append(v)

    check_call(cmd)


def create_pool(service, name, replicas=3, pg_num=None):
    """Create a new RADOS pool."""
    if pool_exists(service, name):
        log("Ceph pool {} already exists, skipping creation".format(name),
            level=WARNING)
        return

    if not pg_num:
        # Calculate the number of placement groups based
        # on upstream recommended best practices.
        osds = get_osds(service)
        if osds:
            pg_num = (len(osds) * 100 // replicas)
        else:
            # NOTE(james-page): Default to 200 for older ceph versions
            # which don't support OSD query from cli
            pg_num = 200

    cmd = ['ceph', '--id', service, 'osd', 'pool', 'create', name, str(pg_num)]
    check_call(cmd)

    update_pool(service, name, settings={'size': str(replicas)})


def delete_pool(service, name):
    """Delete a RADOS pool from ceph."""
    cmd = ['ceph', '--id', service, 'osd', 'pool', 'delete', name,
           '--yes-i-really-really-mean-it']
    check_call(cmd)


def _keyfile_path(service):
    return KEYFILE.format(service)


def _keyring_path(service):
    return KEYRING.format(service)


def create_keyring(service, key):
    """Create a new Ceph keyring containing key."""
    keyring = _keyring_path(service)
    if os.path.exists(keyring):
        log('Ceph keyring exists at %s.' % keyring, level=WARNING)
        return

    cmd = ['ceph-authtool', keyring, '--create-keyring',
           '--name=client.{}'.format(service), '--add-key={}'.format(key)]
    check_call(cmd)
    log('Created new ceph keyring at %s.' % keyring, level=DEBUG)


def delete_keyring(service):
    """Delete an existing Ceph keyring."""
    keyring = _keyring_path(service)
    if not os.path.exists(keyring):
        log('Keyring does not exist at %s' % keyring, level=WARNING)
        return

    os.remove(keyring)
    log('Deleted ring at %s.' % keyring, level=INFO)


def create_key_file(service, key):
    """Create a file containing key."""
    keyfile = _keyfile_path(service)
    if os.path.exists(keyfile):
        log('Keyfile exists at %s.' % keyfile, level=WARNING)
        return

    with open(keyfile, 'w') as fd:
        fd.write(key)

    log('Created new keyfile at %s.' % keyfile, level=INFO)


def get_ceph_nodes(relation='ceph'):
    """Query named relation to determine current nodes."""
    hosts = []
    for r_id in relation_ids(relation):
        for unit in related_units(r_id):
            hosts.append(relation_get('private-address', unit=unit, rid=r_id))

    return hosts


def configure(service, key, auth, use_syslog):
    """Perform basic configuration of Ceph."""
    create_keyring(service, key)
    create_key_file(service, key)
    hosts = get_ceph_nodes()
    with open('/etc/ceph/ceph.conf', 'w') as ceph_conf:
        ceph_conf.write(CEPH_CONF.format(auth=auth,
                                         keyring=_keyring_path(service),
                                         mon_hosts=",".join(map(str, hosts)),
                                         use_syslog=use_syslog))
    modprobe('rbd')


def image_mapped(name):
    """Determine whether a RADOS block device is mapped locally."""
    try:
        out = check_output(['rbd', 'showmapped']).decode('UTF-8')
    except CalledProcessError:
        return False

    return name in out


def map_block_storage(service, pool, image):
    """Map a RADOS block device for local use."""
    cmd = [
        'rbd',
        'map',
        '{}/{}'.format(pool, image),
        '--user',
        service,
        '--secret',
        _keyfile_path(service),
    ]
    check_call(cmd)


def filesystem_mounted(fs):
    """Determine whether a filesytems is already mounted."""
    return fs in [f for f, m in mounts()]


def make_filesystem(blk_device, fstype='ext4', timeout=10):
    """Make a new filesystem on the specified block device."""
    count = 0
    e_noent = os.errno.ENOENT
    while not os.path.exists(blk_device):
        if count >= timeout:
            log('Gave up waiting on block device %s' % blk_device,
                level=ERROR)
            raise IOError(e_noent, os.strerror(e_noent), blk_device)

        log('Waiting for block device %s to appear' % blk_device,
            level=DEBUG)
        count += 1
        time.sleep(1)
    else:
        log('Formatting block device %s as filesystem %s.' %
            (blk_device, fstype), level=INFO)
        check_call(['mkfs', '-t', fstype, blk_device])


def place_data_on_block_device(blk_device, data_src_dst):
    """Migrate data in data_src_dst to blk_device and then remount."""
    # mount block device into /mnt
    mount(blk_device, '/mnt')
    # copy data to /mnt
    copy_files(data_src_dst, '/mnt')
    # umount block device
    umount('/mnt')
    # Grab user/group ID's from original source
    _dir = os.stat(data_src_dst)
    uid = _dir.st_uid
    gid = _dir.st_gid
    # re-mount where the data should originally be
    # TODO: persist is currently a NO-OP in core.host
    mount(blk_device, data_src_dst, persist=True)
    # ensure original ownership of new mount.
    os.chown(data_src_dst, uid, gid)


def copy_files(src, dst, symlinks=False, ignore=None):
    """Copy files from src to dst."""
    for item in os.listdir(src):
        s = os.path.join(src, item)
        d = os.path.join(dst, item)
        if os.path.isdir(s):
            shutil.copytree(s, d, symlinks, ignore)
        else:
            shutil.copy2(s, d)


def ensure_ceph_storage(service, pool, rbd_img, sizemb, mount_point,
                        blk_device, fstype, system_services=[],
                        replicas=3):
    """NOTE: This function must only be called from a single service unit for
    the same rbd_img otherwise data loss will occur.

    Ensures given pool and RBD image exists, is mapped to a block device,
    and the device is formatted and mounted at the given mount_point.

    If formatting a device for the first time, data existing at mount_point
    will be migrated to the RBD device before being re-mounted.

    All services listed in system_services will be stopped prior to data
    migration and restarted when complete.
    """
    # Ensure pool, RBD image, RBD mappings are in place.
    if not pool_exists(service, pool):
        log('Creating new pool {}.'.format(pool), level=INFO)
        create_pool(service, pool, replicas=replicas)

    if not rbd_exists(service, pool, rbd_img):
        log('Creating RBD image ({}).'.format(rbd_img), level=INFO)
        create_rbd_image(service, pool, rbd_img, sizemb)

    if not image_mapped(rbd_img):
        log('Mapping RBD Image {} as a Block Device.'.format(rbd_img),
            level=INFO)
        map_block_storage(service, pool, rbd_img)

    # make file system
    # TODO: What happens if for whatever reason this is run again and
    # the data is already in the rbd device and/or is mounted??
    # When it is mounted already, it will fail to make the fs
    # XXX: This is really sketchy!  Need to at least add an fstab entry
    #      otherwise this hook will blow away existing data if its executed
    #      after a reboot.
    if not filesystem_mounted(mount_point):
        make_filesystem(blk_device, fstype)

        for svc in system_services:
            if service_running(svc):
                log('Stopping services {} prior to migrating data.'
                    .format(svc), level=DEBUG)
                service_stop(svc)

        place_data_on_block_device(blk_device, mount_point)

        for svc in system_services:
            log('Starting service {} after migrating data.'
                .format(svc), level=DEBUG)
            service_start(svc)


def ensure_ceph_keyring(service, user=None, group=None, relation='ceph'):
    """Ensures a ceph keyring is created for a named service and optionally
    ensures user and group ownership.

    Returns False if no ceph key is available in relation state.
    """
    key = None
    for rid in relation_ids(relation):
        for unit in related_units(rid):
            key = relation_get('key', rid=rid, unit=unit)
            if key:
                break

    if not key:
        return False

    create_keyring(service=service, key=key)
    keyring = _keyring_path(service)
    if user and group:
        check_call(['chown', '%s.%s' % (user, group), keyring])

    return True


def ceph_version():
    """Retrieve the local version of ceph."""
    if os.path.exists('/usr/bin/ceph'):
        cmd = ['ceph', '-v']
        output = check_output(cmd).decode('US-ASCII')
        output = output.split()
        if len(output) > 3:
            return output[2]
        else:
            return None
    else:
        return None


class CephBrokerRq(object):
    """Ceph broker request.

    Multiple operations can be added to a request and sent to the Ceph broker
    to be executed.

    Request is json-encoded for sending over the wire.

    The API is versioned and defaults to version 1.
    """

    def __init__(self, api_version=1, request_id=None):
        self.api_version = api_version
        if request_id:
            self.request_id = request_id
        else:
            self.request_id = str(uuid.uuid1())
        self.ops = []

    def add_op_create_pool(self, name, replica_count=3, pg_num=None,
                           weight=None):
        """Adds an operation to create a pool.

        @param pg_num setting:  optional setting. If not provided, this value
        will be calculated by the broker based on how many OSDs are in the
        cluster at the time of creation. Note that, if provided, this value
        will be capped at the current available maximum.
        @param weight: the percentage of data the pool makes up
        """
        if pg_num and weight:
            raise ValueError('pg_num and weight are mutually exclusive')

        self.ops.append({'op': 'create-pool', 'name': name,
                         'replicas': replica_count, 'pg_num': pg_num,
                         'weight': weight})

    def set_ops(self, ops):
        """Set request ops to provided value.

        Useful for injecting ops that come from a previous request
        to allow comparisons to ensure validity.
        """
        self.ops = ops

    @property
    def request(self):
        return json.dumps({'api-version': self.api_version, 'ops': self.ops,
                           'request-id': self.request_id})

    def _ops_equal(self, other):
        if len(self.ops) == len(other.ops):
            for req_no in range(0, len(self.ops)):
                for key in ['replicas', 'name', 'op', 'pg_num', 'weight']:
                    if self.ops[req_no].get(key) != other.ops[req_no].get(key):
                        return False
        else:
            return False
        return True

    def __eq__(self, other):
        if not isinstance(other, self.__class__):
            return False
        if self.api_version == other.api_version and \
                self._ops_equal(other):
            return True
        else:
            return False

    def __ne__(self, other):
        return not self.__eq__(other)


class CephBrokerRsp(object):
    """Ceph broker response.

    Response is json-decoded and contents provided as methods/properties.

    The API is versioned and defaults to version 1.
    """

    def __init__(self, encoded_rsp):
        self.api_version = None
        self.rsp = json.loads(encoded_rsp)

    @property
    def request_id(self):
        return self.rsp.get('request-id')

    @property
    def exit_code(self):
        return self.rsp.get('exit-code')

    @property
    def exit_msg(self):
        return self.rsp.get('stderr')


# Ceph Broker Conversation:
# If a charm needs an action to be taken by ceph it can create a CephBrokerRq
# and send that request to ceph via the ceph relation. The CephBrokerRq has a
# unique id so that the client can identity which CephBrokerRsp is associated
# with the request. Ceph will also respond to each client unit individually
# creating a response key per client unit eg glance/0 will get a CephBrokerRsp
# via key broker-rsp-glance-0
#
# To use this the charm can just do something like:
#
# from charmhelpers.contrib.storage.linux.ceph import (
#     send_request_if_needed,
#     is_request_complete,
#     CephBrokerRq,
# )
#
# @hooks.hook('ceph-relation-changed')
# def ceph_changed():
#     rq = CephBrokerRq()
#     rq.add_op_create_pool(name='poolname', replica_count=3)
#
#     if is_request_complete(rq):
#         <Request complete actions>
#     else:
#         send_request_if_needed(get_ceph_request())
#
# CephBrokerRq and CephBrokerRsp are serialized into JSON. Below is an example
# of glance having sent a request to ceph which ceph has successfully processed
#  'ceph:8': {
#      'ceph/0': {
#          'auth': 'cephx',
#          'broker-rsp-glance-0': '{"request-id": "0bc7dc54", "exit-code": 0}',
#          'broker_rsp': '{"request-id": "0da543b8", "exit-code": 0}',
#          'ceph-public-address': '10.5.44.103',
#          'key': 'AQCLDttVuHXINhAAvI144CB09dYchhHyTUY9BQ==',
#          'private-address': '10.5.44.103',
#      },
#      'glance/0': {
#          'broker_req': ('{"api-version": 1, "request-id": "0bc7dc54", '
#                         '"ops": [{"replicas": 3, "name": "glance", '
#                         '"op": "create-pool"}]}'),
#          'private-address': '10.5.44.109',
#      },
#  }

def get_previous_request(rid):
    """Return the last ceph broker request sent on a given relation

    @param rid: Relation id to query for request
    """
    request = None
    broker_req = relation_get(attribute='broker_req', rid=rid,
                              unit=local_unit())
    if broker_req:
        request_data = json.loads(broker_req)
        request = CephBrokerRq(api_version=request_data['api-version'],
                               request_id=request_data['request-id'])
        request.set_ops(request_data['ops'])

    return request


def get_request_states(request, relation='ceph'):
    """Return a dict of requests per relation id with their corresponding
       completion state.

    This allows a charm, which has a request for ceph, to see whether there is
    an equivalent request already being processed and if so what state that
    request is in.

    @param request: A CephBrokerRq object
    """
    complete = []
    requests = {}
    for rid in relation_ids(relation):
        complete = False
        previous_request = get_previous_request(rid)
        if request == previous_request:
            sent = True
            complete = is_request_complete_for_rid(previous_request, rid)
        else:
            sent = False
            complete = False

        requests[rid] = {
            'sent': sent,
            'complete': complete,
        }

    return requests


def is_request_sent(request, relation='ceph'):
    """Check to see if a functionally equivalent request has already been sent

    Returns True if a similair request has been sent

    @param request: A CephBrokerRq object
    """
    states = get_request_states(request, relation=relation)
    for rid in states.keys():
        if not states[rid]['sent']:
            return False

    return True


def is_request_complete(request, relation='ceph'):
    """Check to see if a functionally equivalent request has already been
    completed

    Returns True if a similair request has been completed

    @param request: A CephBrokerRq object
    """
    states = get_request_states(request, relation=relation)
    for rid in states.keys():
        if not states[rid]['complete']:
            return False

    return True


def is_request_complete_for_rid(request, rid):
    """Check if a given request has been completed on the given relation

    @param request: A CephBrokerRq object
    @param rid: Relation ID
    """
    broker_key = get_broker_rsp_key()
    for unit in related_units(rid):
        rdata = relation_get(rid=rid, unit=unit)
        if rdata.get(broker_key):
            rsp = CephBrokerRsp(rdata.get(broker_key))
            if rsp.request_id == request.request_id:
                if not rsp.exit_code:
                    return True
        else:
            # The remote unit sent no reply targeted at this unit so either the
            # remote ceph cluster does not support unit targeted replies or it
            # has not processed our request yet.
            if rdata.get('broker_rsp'):
                request_data = json.loads(rdata['broker_rsp'])
                if request_data.get('request-id'):
                    log('Ignoring legacy broker_rsp without unit key as remote '
                        'service supports unit specific replies', level=DEBUG)
                else:
                    log('Using legacy broker_rsp as remote service does not '
                        'supports unit specific replies', level=DEBUG)
                    rsp = CephBrokerRsp(rdata['broker_rsp'])
                    if not rsp.exit_code:
                        return True

    return False


def get_broker_rsp_key():
    """Return broker response key for this unit

    This is the key that ceph is going to use to pass request status
    information back to this unit
    """
    return 'broker-rsp-' + local_unit().replace('/', '-')


def send_request_if_needed(request, relation='ceph'):
    """Send broker request if an equivalent request has not already been sent

    @param request: A CephBrokerRq object
    """
    if is_request_sent(request, relation=relation):
        log('Request already sent but not complete, not sending new request',
            level=DEBUG)
    else:
        for rid in relation_ids(relation):
            log('Sending request {}'.format(request.request_id), level=DEBUG)
            relation_set(relation_id=rid, broker_req=request.request)


class CephConfContext(object):
    """Ceph config (ceph.conf) context.

    Supports user-provided Ceph configuration settings. Use can provide a
    dictionary as the value for the config-flags charm option containing
    Ceph configuration settings keyede by their section in ceph.conf.
    """
    def __init__(self, permitted_sections=None):
        self.permitted_sections = permitted_sections or []

    def __call__(self):
        conf = config('config-flags')
        if not conf:
            return {}

        conf = config_flags_parser(conf)
        if type(conf) != dict:
            log("Provided config-flags is not a dictionary - ignoring",
                level=WARNING)
            return {}

        permitted = self.permitted_sections
        if permitted:
            diff = set(conf.keys()).difference(set(permitted))
            if diff:
                log("Config-flags contains invalid keys '%s' - they will be "
                    "ignored" % (', '.join(diff)), level=WARNING)

        ceph_conf = {}
        for key in conf:
            if permitted and key not in permitted:
                log("Ignoring key '%s'" % key, level=WARNING)
                continue

            ceph_conf[key] = conf[key]

        return ceph_conf
