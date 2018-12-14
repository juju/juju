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

# Common python helper functions used for OpenStack charms.
from collections import OrderedDict
from functools import wraps

import subprocess
import json
import os
import sys
import re
import itertools
import functools

import six
import traceback
import uuid
import yaml

from charmhelpers import deprecate

from charmhelpers.contrib.network import ip

from charmhelpers.core import unitdata

from charmhelpers.core.hookenv import (
    action_fail,
    action_set,
    config,
    log as juju_log,
    charm_dir,
    INFO,
    ERROR,
    related_units,
    relation_ids,
    relation_set,
    status_set,
    hook_name,
    application_version_set,
    cached,
)

from charmhelpers.core.strutils import BasicStringComparator

from charmhelpers.contrib.storage.linux.lvm import (
    deactivate_lvm_volume_group,
    is_lvm_physical_volume,
    remove_lvm_physical_volume,
)

from charmhelpers.contrib.network.ip import (
    get_ipv6_addr,
    is_ipv6,
    port_has_listener,
)

from charmhelpers.core.host import (
    lsb_release,
    mounts,
    umount,
    service_running,
    service_pause,
    service_resume,
    restart_on_change_helper,
)
from charmhelpers.fetch import (
    apt_cache,
    import_key as fetch_import_key,
    add_source as fetch_add_source,
    SourceConfigError,
    GPGKeyError,
    get_upstream_version
)

from charmhelpers.fetch.snap import (
    snap_install,
    snap_refresh,
    valid_snap_channel,
)

from charmhelpers.contrib.storage.linux.utils import is_block_device, zap_disk
from charmhelpers.contrib.storage.linux.loopback import ensure_loopback_device
from charmhelpers.contrib.openstack.exceptions import OSContextError

CLOUD_ARCHIVE_URL = "http://ubuntu-cloud.archive.canonical.com/ubuntu"
CLOUD_ARCHIVE_KEY_ID = '5EDB1B62EC4926EA'

DISTRO_PROPOSED = ('deb http://archive.ubuntu.com/ubuntu/ %s-proposed '
                   'restricted main multiverse universe')

OPENSTACK_RELEASES = (
    'diablo',
    'essex',
    'folsom',
    'grizzly',
    'havana',
    'icehouse',
    'juno',
    'kilo',
    'liberty',
    'mitaka',
    'newton',
    'ocata',
    'pike',
    'queens',
    'rocky',
)

UBUNTU_OPENSTACK_RELEASE = OrderedDict([
    ('oneiric', 'diablo'),
    ('precise', 'essex'),
    ('quantal', 'folsom'),
    ('raring', 'grizzly'),
    ('saucy', 'havana'),
    ('trusty', 'icehouse'),
    ('utopic', 'juno'),
    ('vivid', 'kilo'),
    ('wily', 'liberty'),
    ('xenial', 'mitaka'),
    ('yakkety', 'newton'),
    ('zesty', 'ocata'),
    ('artful', 'pike'),
    ('bionic', 'queens'),
    ('cosmic', 'rocky'),
])


OPENSTACK_CODENAMES = OrderedDict([
    ('2011.2', 'diablo'),
    ('2012.1', 'essex'),
    ('2012.2', 'folsom'),
    ('2013.1', 'grizzly'),
    ('2013.2', 'havana'),
    ('2014.1', 'icehouse'),
    ('2014.2', 'juno'),
    ('2015.1', 'kilo'),
    ('2015.2', 'liberty'),
    ('2016.1', 'mitaka'),
    ('2016.2', 'newton'),
    ('2017.1', 'ocata'),
    ('2017.2', 'pike'),
    ('2018.1', 'queens'),
    ('2018.2', 'rocky'),
])

# The ugly duckling - must list releases oldest to newest
SWIFT_CODENAMES = OrderedDict([
    ('diablo',
        ['1.4.3']),
    ('essex',
        ['1.4.8']),
    ('folsom',
        ['1.7.4']),
    ('grizzly',
        ['1.7.6', '1.7.7', '1.8.0']),
    ('havana',
        ['1.9.0', '1.9.1', '1.10.0']),
    ('icehouse',
        ['1.11.0', '1.12.0', '1.13.0', '1.13.1']),
    ('juno',
        ['2.0.0', '2.1.0', '2.2.0']),
    ('kilo',
        ['2.2.1', '2.2.2']),
    ('liberty',
        ['2.3.0', '2.4.0', '2.5.0']),
    ('mitaka',
        ['2.5.0', '2.6.0', '2.7.0']),
    ('newton',
        ['2.8.0', '2.9.0', '2.10.0']),
    ('ocata',
        ['2.11.0', '2.12.0', '2.13.0']),
    ('pike',
        ['2.13.0', '2.15.0']),
    ('queens',
        ['2.16.0', '2.17.0']),
    ('rocky',
        ['2.18.0', '2.19.0']),
])

# >= Liberty version->codename mapping
PACKAGE_CODENAMES = {
    'nova-common': OrderedDict([
        ('12', 'liberty'),
        ('13', 'mitaka'),
        ('14', 'newton'),
        ('15', 'ocata'),
        ('16', 'pike'),
        ('17', 'queens'),
        ('18', 'rocky'),
    ]),
    'neutron-common': OrderedDict([
        ('7', 'liberty'),
        ('8', 'mitaka'),
        ('9', 'newton'),
        ('10', 'ocata'),
        ('11', 'pike'),
        ('12', 'queens'),
        ('13', 'rocky'),
    ]),
    'cinder-common': OrderedDict([
        ('7', 'liberty'),
        ('8', 'mitaka'),
        ('9', 'newton'),
        ('10', 'ocata'),
        ('11', 'pike'),
        ('12', 'queens'),
        ('13', 'rocky'),
    ]),
    'keystone': OrderedDict([
        ('8', 'liberty'),
        ('9', 'mitaka'),
        ('10', 'newton'),
        ('11', 'ocata'),
        ('12', 'pike'),
        ('13', 'queens'),
        ('14', 'rocky'),
    ]),
    'horizon-common': OrderedDict([
        ('8', 'liberty'),
        ('9', 'mitaka'),
        ('10', 'newton'),
        ('11', 'ocata'),
        ('12', 'pike'),
        ('13', 'queens'),
        ('14', 'rocky'),
    ]),
    'ceilometer-common': OrderedDict([
        ('5', 'liberty'),
        ('6', 'mitaka'),
        ('7', 'newton'),
        ('8', 'ocata'),
        ('9', 'pike'),
        ('10', 'queens'),
        ('11', 'rocky'),
    ]),
    'heat-common': OrderedDict([
        ('5', 'liberty'),
        ('6', 'mitaka'),
        ('7', 'newton'),
        ('8', 'ocata'),
        ('9', 'pike'),
        ('10', 'queens'),
        ('11', 'rocky'),
    ]),
    'glance-common': OrderedDict([
        ('11', 'liberty'),
        ('12', 'mitaka'),
        ('13', 'newton'),
        ('14', 'ocata'),
        ('15', 'pike'),
        ('16', 'queens'),
        ('17', 'rocky'),
    ]),
    'openstack-dashboard': OrderedDict([
        ('8', 'liberty'),
        ('9', 'mitaka'),
        ('10', 'newton'),
        ('11', 'ocata'),
        ('12', 'pike'),
        ('13', 'queens'),
        ('14', 'rocky'),
    ]),
}

DEFAULT_LOOPBACK_SIZE = '5G'


class CompareOpenStackReleases(BasicStringComparator):
    """Provide comparisons of OpenStack releases.

    Use in the form of

    if CompareOpenStackReleases(release) > 'mitaka':
        # do something with mitaka
    """
    _list = OPENSTACK_RELEASES


def error_out(msg):
    juju_log("FATAL ERROR: %s" % msg, level='ERROR')
    sys.exit(1)


def get_os_codename_install_source(src):
    '''Derive OpenStack release codename from a given installation source.'''
    ubuntu_rel = lsb_release()['DISTRIB_CODENAME']
    rel = ''
    if src is None:
        return rel
    if src in ['distro', 'distro-proposed']:
        try:
            rel = UBUNTU_OPENSTACK_RELEASE[ubuntu_rel]
        except KeyError:
            e = 'Could not derive openstack release for '\
                'this Ubuntu release: %s' % ubuntu_rel
            error_out(e)
        return rel

    if src.startswith('cloud:'):
        ca_rel = src.split(':')[1]
        ca_rel = ca_rel.split('-')[1].split('/')[0]
        return ca_rel

    # Best guess match based on deb string provided
    if (src.startswith('deb') or
            src.startswith('ppa') or
            src.startswith('snap')):
        for v in OPENSTACK_CODENAMES.values():
            if v in src:
                return v


def get_os_version_install_source(src):
    codename = get_os_codename_install_source(src)
    return get_os_version_codename(codename)


def get_os_codename_version(vers):
    '''Determine OpenStack codename from version number.'''
    try:
        return OPENSTACK_CODENAMES[vers]
    except KeyError:
        e = 'Could not determine OpenStack codename for version %s' % vers
        error_out(e)


def get_os_version_codename(codename, version_map=OPENSTACK_CODENAMES):
    '''Determine OpenStack version number from codename.'''
    for k, v in six.iteritems(version_map):
        if v == codename:
            return k
    e = 'Could not derive OpenStack version for '\
        'codename: %s' % codename
    error_out(e)


def get_os_version_codename_swift(codename):
    '''Determine OpenStack version number of swift from codename.'''
    for k, v in six.iteritems(SWIFT_CODENAMES):
        if k == codename:
            return v[-1]
    e = 'Could not derive swift version for '\
        'codename: %s' % codename
    error_out(e)


def get_swift_codename(version):
    '''Determine OpenStack codename that corresponds to swift version.'''
    codenames = [k for k, v in six.iteritems(SWIFT_CODENAMES) if version in v]

    if len(codenames) > 1:
        # If more than one release codename contains this version we determine
        # the actual codename based on the highest available install source.
        for codename in reversed(codenames):
            releases = UBUNTU_OPENSTACK_RELEASE
            release = [k for k, v in six.iteritems(releases) if codename in v]
            ret = subprocess.check_output(['apt-cache', 'policy', 'swift'])
            if six.PY3:
                ret = ret.decode('UTF-8')
            if codename in ret or release[0] in ret:
                return codename
    elif len(codenames) == 1:
        return codenames[0]

    # NOTE: fallback - attempt to match with just major.minor version
    match = re.match(r'^(\d+)\.(\d+)', version)
    if match:
        major_minor_version = match.group(0)
        for codename, versions in six.iteritems(SWIFT_CODENAMES):
            for release_version in versions:
                if release_version.startswith(major_minor_version):
                    return codename

    return None


def get_os_codename_package(package, fatal=True):
    '''Derive OpenStack release codename from an installed package.'''

    if snap_install_requested():
        cmd = ['snap', 'list', package]
        try:
            out = subprocess.check_output(cmd)
            if six.PY3:
                out = out.decode('UTF-8')
        except subprocess.CalledProcessError:
            return None
        lines = out.split('\n')
        for line in lines:
            if package in line:
                # Second item in list is Version
                return line.split()[1]

    import apt_pkg as apt

    cache = apt_cache()

    try:
        pkg = cache[package]
    except Exception:
        if not fatal:
            return None
        # the package is unknown to the current apt cache.
        e = 'Could not determine version of package with no installation '\
            'candidate: %s' % package
        error_out(e)

    if not pkg.current_ver:
        if not fatal:
            return None
        # package is known, but no version is currently installed.
        e = 'Could not determine version of uninstalled package: %s' % package
        error_out(e)

    vers = apt.upstream_version(pkg.current_ver.ver_str)
    if 'swift' in pkg.name:
        # Fully x.y.z match for swift versions
        match = re.match(r'^(\d+)\.(\d+)\.(\d+)', vers)
    else:
        # x.y match only for 20XX.X
        # and ignore patch level for other packages
        match = re.match(r'^(\d+)\.(\d+)', vers)

    if match:
        vers = match.group(0)

    # Generate a major version number for newer semantic
    # versions of openstack projects
    major_vers = vers.split('.')[0]
    # >= Liberty independent project versions
    if (package in PACKAGE_CODENAMES and
            major_vers in PACKAGE_CODENAMES[package]):
        return PACKAGE_CODENAMES[package][major_vers]
    else:
        # < Liberty co-ordinated project versions
        try:
            if 'swift' in pkg.name:
                return get_swift_codename(vers)
            else:
                return OPENSTACK_CODENAMES[vers]
        except KeyError:
            if not fatal:
                return None
            e = 'Could not determine OpenStack codename for version %s' % vers
            error_out(e)


def get_os_version_package(pkg, fatal=True):
    '''Derive OpenStack version number from an installed package.'''
    codename = get_os_codename_package(pkg, fatal=fatal)

    if not codename:
        return None

    if 'swift' in pkg:
        vers_map = SWIFT_CODENAMES
        for cname, version in six.iteritems(vers_map):
            if cname == codename:
                return version[-1]
    else:
        vers_map = OPENSTACK_CODENAMES
        for version, cname in six.iteritems(vers_map):
            if cname == codename:
                return version
    # e = "Could not determine OpenStack version for package: %s" % pkg
    # error_out(e)


# Module local cache variable for the os_release.
_os_rel = None


def reset_os_release():
    '''Unset the cached os_release version'''
    global _os_rel
    _os_rel = None


def os_release(package, base='essex', reset_cache=False):
    '''
    Returns OpenStack release codename from a cached global.

    If reset_cache then unset the cached os_release version and return the
    freshly determined version.

    If the codename can not be determined from either an installed package or
    the installation source, the earliest release supported by the charm should
    be returned.
    '''
    global _os_rel
    if reset_cache:
        reset_os_release()
    if _os_rel:
        return _os_rel
    _os_rel = (
        get_os_codename_package(package, fatal=False) or
        get_os_codename_install_source(config('openstack-origin')) or
        base)
    return _os_rel


@deprecate("moved to charmhelpers.fetch.import_key()", "2017-07", log=juju_log)
def import_key(keyid):
    """Import a key, either ASCII armored, or a GPG key id.

    @param keyid: the key in ASCII armor format, or a GPG key id.
    @raises SystemExit() via sys.exit() on failure.
    """
    try:
        return fetch_import_key(keyid)
    except GPGKeyError as e:
        error_out("Could not import key: {}".format(str(e)))


def get_source_and_pgp_key(source_and_key):
    """Look for a pgp key ID or ascii-armor key in the given input.

    :param source_and_key: Sting, "source_spec|keyid" where '|keyid' is
        optional.
    :returns (source_spec, key_id OR None) as a tuple.  Returns None for key_id
        if there was no '|' in the source_and_key string.
    """
    try:
        source, key = source_and_key.split('|', 2)
        return source, key or None
    except ValueError:
        return source_and_key, None


@deprecate("use charmhelpers.fetch.add_source() instead.",
           "2017-07", log=juju_log)
def configure_installation_source(source_plus_key):
    """Configure an installation source.

    The functionality is provided by charmhelpers.fetch.add_source()
    The difference between the two functions is that add_source() signature
    requires the key to be passed directly, whereas this function passes an
    optional key by appending '|<key>' to the end of the source specificiation
    'source'.

    Another difference from add_source() is that the function calls sys.exit(1)
    if the configuration fails, whereas add_source() raises
    SourceConfigurationError().  Another difference, is that add_source()
    silently fails (with a juju_log command) if there is no matching source to
    configure, whereas this function fails with a sys.exit(1)

    :param source: String_plus_key -- see above for details.

    Note that the behaviour on error is to log the error to the juju log and
    then call sys.exit(1).
    """
    if source_plus_key.startswith('snap'):
        # Do nothing for snap installs
        return
    # extract the key if there is one, denoted by a '|' in the rel
    source, key = get_source_and_pgp_key(source_plus_key)

    # handle the ordinary sources via add_source
    try:
        fetch_add_source(source, key, fail_invalid=True)
    except SourceConfigError as se:
        error_out(str(se))


def config_value_changed(option):
    """
    Determine if config value changed since last call to this function.
    """
    hook_data = unitdata.HookData()
    with hook_data():
        db = unitdata.kv()
        current = config(option)
        saved = db.get(option)
        db.set(option, current)
        if saved is None:
            return False
        return current != saved


def save_script_rc(script_path="scripts/scriptrc", **env_vars):
    """
    Write an rc file in the charm-delivered directory containing
    exported environment variables provided by env_vars. Any charm scripts run
    outside the juju hook environment can source this scriptrc to obtain
    updated config information necessary to perform health checks or
    service changes.
    """
    juju_rc_path = "%s/%s" % (charm_dir(), script_path)
    if not os.path.exists(os.path.dirname(juju_rc_path)):
        os.mkdir(os.path.dirname(juju_rc_path))
    with open(juju_rc_path, 'wt') as rc_script:
        rc_script.write(
            "#!/bin/bash\n")
        [rc_script.write('export %s=%s\n' % (u, p))
         for u, p in six.iteritems(env_vars) if u != "script_path"]


def openstack_upgrade_available(package):
    """
    Determines if an OpenStack upgrade is available from installation
    source, based on version of installed package.

    :param package: str: Name of installed package.

    :returns: bool:    : Returns True if configured installation source offers
                         a newer version of package.
    """

    import apt_pkg as apt
    src = config('openstack-origin')
    cur_vers = get_os_version_package(package)
    if not cur_vers:
        # The package has not been installed yet do not attempt upgrade
        return False
    if "swift" in package:
        codename = get_os_codename_install_source(src)
        avail_vers = get_os_version_codename_swift(codename)
    else:
        avail_vers = get_os_version_install_source(src)
    apt.init()
    return apt.version_compare(avail_vers, cur_vers) == 1


def ensure_block_device(block_device):
    '''
    Confirm block_device, create as loopback if necessary.

    :param block_device: str: Full path of block device to ensure.

    :returns: str: Full path of ensured block device.
    '''
    _none = ['None', 'none', None]
    if (block_device in _none):
        error_out('prepare_storage(): Missing required input: block_device=%s.'
                  % block_device)

    if block_device.startswith('/dev/'):
        bdev = block_device
    elif block_device.startswith('/'):
        _bd = block_device.split('|')
        if len(_bd) == 2:
            bdev, size = _bd
        else:
            bdev = block_device
            size = DEFAULT_LOOPBACK_SIZE
        bdev = ensure_loopback_device(bdev, size)
    else:
        bdev = '/dev/%s' % block_device

    if not is_block_device(bdev):
        error_out('Failed to locate valid block device at %s' % bdev)

    return bdev


def clean_storage(block_device):
    '''
    Ensures a block device is clean.  That is:
        - unmounted
        - any lvm volume groups are deactivated
        - any lvm physical device signatures removed
        - partition table wiped

    :param block_device: str: Full path to block device to clean.
    '''
    for mp, d in mounts():
        if d == block_device:
            juju_log('clean_storage(): %s is mounted @ %s, unmounting.' %
                     (d, mp), level=INFO)
            umount(mp, persist=True)

    if is_lvm_physical_volume(block_device):
        deactivate_lvm_volume_group(block_device)
        remove_lvm_physical_volume(block_device)
    else:
        zap_disk(block_device)


is_ip = ip.is_ip
ns_query = ip.ns_query
get_host_ip = ip.get_host_ip
get_hostname = ip.get_hostname


def get_matchmaker_map(mm_file='/etc/oslo/matchmaker_ring.json'):
    mm_map = {}
    if os.path.isfile(mm_file):
        with open(mm_file, 'r') as f:
            mm_map = json.load(f)
    return mm_map


def sync_db_with_multi_ipv6_addresses(database, database_user,
                                      relation_prefix=None):
    hosts = get_ipv6_addr(dynamic_only=False)

    if config('vip'):
        vips = config('vip').split()
        for vip in vips:
            if vip and is_ipv6(vip):
                hosts.append(vip)

    kwargs = {'database': database,
              'username': database_user,
              'hostname': json.dumps(hosts)}

    if relation_prefix:
        for key in list(kwargs.keys()):
            kwargs["%s_%s" % (relation_prefix, key)] = kwargs[key]
            del kwargs[key]

    for rid in relation_ids('shared-db'):
        relation_set(relation_id=rid, **kwargs)


def os_requires_version(ostack_release, pkg):
    """
    Decorator for hook to specify minimum supported release
    """
    def wrap(f):
        @wraps(f)
        def wrapped_f(*args):
            if os_release(pkg) < ostack_release:
                raise Exception("This hook is not supported on releases"
                                " before %s" % ostack_release)
            f(*args)
        return wrapped_f
    return wrap


def os_workload_status(configs, required_interfaces, charm_func=None):
    """
    Decorator to set workload status based on complete contexts
    """
    def wrap(f):
        @wraps(f)
        def wrapped_f(*args, **kwargs):
            # Run the original function first
            f(*args, **kwargs)
            # Set workload status now that contexts have been
            # acted on
            set_os_workload_status(configs, required_interfaces, charm_func)
        return wrapped_f
    return wrap


def set_os_workload_status(configs, required_interfaces, charm_func=None,
                           services=None, ports=None):
    """Set the state of the workload status for the charm.

    This calls _determine_os_workload_status() to get the new state, message
    and sets the status using status_set()

    @param configs: a templating.OSConfigRenderer() object
    @param required_interfaces: {generic: [specific, specific2, ...]}
    @param charm_func: a callable function that returns state, message. The
                       signature is charm_func(configs) -> (state, message)
    @param services: list of strings OR dictionary specifying services/ports
    @param ports: OPTIONAL list of port numbers.
    @returns state, message: the new workload status, user message
    """
    state, message = _determine_os_workload_status(
        configs, required_interfaces, charm_func, services, ports)
    status_set(state, message)


def _determine_os_workload_status(
        configs, required_interfaces, charm_func=None,
        services=None, ports=None):
    """Determine the state of the workload status for the charm.

    This function returns the new workload status for the charm based
    on the state of the interfaces, the paused state and whether the
    services are actually running and any specified ports are open.

    This checks:

     1. if the unit should be paused, that it is actually paused.  If so the
        state is 'maintenance' + message, else 'broken'.
     2. that the interfaces/relations are complete.  If they are not then
        it sets the state to either 'broken' or 'waiting' and an appropriate
        message.
     3. If all the relation data is set, then it checks that the actual
        services really are running.  If not it sets the state to 'broken'.

    If everything is okay then the state returns 'active'.

    @param configs: a templating.OSConfigRenderer() object
    @param required_interfaces: {generic: [specific, specific2, ...]}
    @param charm_func: a callable function that returns state, message. The
                       signature is charm_func(configs) -> (state, message)
    @param services: list of strings OR dictionary specifying services/ports
    @param ports: OPTIONAL list of port numbers.
    @returns state, message: the new workload status, user message
    """
    state, message = _ows_check_if_paused(services, ports)

    if state is None:
        state, message = _ows_check_generic_interfaces(
            configs, required_interfaces)

    if state != 'maintenance' and charm_func:
        # _ows_check_charm_func() may modify the state, message
        state, message = _ows_check_charm_func(
            state, message, lambda: charm_func(configs))

    if state is None:
        state, message = _ows_check_services_running(services, ports)

    if state is None:
        state = 'active'
        message = "Unit is ready"
        juju_log(message, 'INFO')

    return state, message


def _ows_check_if_paused(services=None, ports=None):
    """Check if the unit is supposed to be paused, and if so check that the
    services/ports (if passed) are actually stopped/not being listened to.

    If the unit isn't supposed to be paused, just return None, None

    If the unit is performing a series upgrade, return a message indicating
    this.

    @param services: OPTIONAL services spec or list of service names.
    @param ports: OPTIONAL list of port numbers.
    @returns state, message or None, None
    """
    if is_unit_upgrading_set():
        state, message = check_actually_paused(services=services,
                                               ports=ports)
        if state is None:
            # we're paused okay, so set maintenance and return
            state = "blocked"
            message = ("Ready for do-release-upgrade and reboot. "
                       "Set complete when finished.")
        return state, message

    if is_unit_paused_set():
        state, message = check_actually_paused(services=services,
                                               ports=ports)
        if state is None:
            # we're paused okay, so set maintenance and return
            state = "maintenance"
            message = "Paused. Use 'resume' action to resume normal service."
        return state, message
    return None, None


def _ows_check_generic_interfaces(configs, required_interfaces):
    """Check the complete contexts to determine the workload status.

     - Checks for missing or incomplete contexts
     - juju log details of missing required data.
     - determines the correct workload status
     - creates an appropriate message for status_set(...)

    if there are no problems then the function returns None, None

    @param configs: a templating.OSConfigRenderer() object
    @params required_interfaces: {generic_interface: [specific_interface], }
    @returns state, message or None, None
    """
    incomplete_rel_data = incomplete_relation_data(configs,
                                                   required_interfaces)
    state = None
    message = None
    missing_relations = set()
    incomplete_relations = set()

    for generic_interface, relations_states in incomplete_rel_data.items():
        related_interface = None
        missing_data = {}
        # Related or not?
        for interface, relation_state in relations_states.items():
            if relation_state.get('related'):
                related_interface = interface
                missing_data = relation_state.get('missing_data')
                break
        # No relation ID for the generic_interface?
        if not related_interface:
            juju_log("{} relation is missing and must be related for "
                     "functionality. ".format(generic_interface), 'WARN')
            state = 'blocked'
            missing_relations.add(generic_interface)
        else:
            # Relation ID eists but no related unit
            if not missing_data:
                # Edge case - relation ID exists but departings
                _hook_name = hook_name()
                if (('departed' in _hook_name or 'broken' in _hook_name) and
                        related_interface in _hook_name):
                    state = 'blocked'
                    missing_relations.add(generic_interface)
                    juju_log("{} relation's interface, {}, "
                             "relationship is departed or broken "
                             "and is required for functionality."
                             "".format(generic_interface, related_interface),
                             "WARN")
                # Normal case relation ID exists but no related unit
                # (joining)
                else:
                    juju_log("{} relations's interface, {}, is related but has"
                             " no units in the relation."
                             "".format(generic_interface, related_interface),
                             "INFO")
            # Related unit exists and data missing on the relation
            else:
                juju_log("{} relation's interface, {}, is related awaiting "
                         "the following data from the relationship: {}. "
                         "".format(generic_interface, related_interface,
                                   ", ".join(missing_data)), "INFO")
            if state != 'blocked':
                state = 'waiting'
            if generic_interface not in missing_relations:
                incomplete_relations.add(generic_interface)

    if missing_relations:
        message = "Missing relations: {}".format(", ".join(missing_relations))
        if incomplete_relations:
            message += "; incomplete relations: {}" \
                       "".format(", ".join(incomplete_relations))
        state = 'blocked'
    elif incomplete_relations:
        message = "Incomplete relations: {}" \
                  "".format(", ".join(incomplete_relations))
        state = 'waiting'

    return state, message


def _ows_check_charm_func(state, message, charm_func_with_configs):
    """Run a custom check function for the charm to see if it wants to
    change the state.  This is only run if not in 'maintenance' and
    tests to see if the new state is more important that the previous
    one determined by the interfaces/relations check.

    @param state: the previously determined state so far.
    @param message: the user orientated message so far.
    @param charm_func: a callable function that returns state, message
    @returns state, message strings.
    """
    if charm_func_with_configs:
        charm_state, charm_message = charm_func_with_configs()
        if charm_state != 'active' and charm_state != 'unknown':
            state = workload_state_compare(state, charm_state)
            if message:
                charm_message = charm_message.replace("Incomplete relations: ",
                                                      "")
                message = "{}, {}".format(message, charm_message)
            else:
                message = charm_message
    return state, message


def _ows_check_services_running(services, ports):
    """Check that the services that should be running are actually running
    and that any ports specified are being listened to.

    @param services: list of strings OR dictionary specifying services/ports
    @param ports: list of ports
    @returns state, message: strings or None, None
    """
    messages = []
    state = None
    if services is not None:
        services = _extract_services_list_helper(services)
        services_running, running = _check_running_services(services)
        if not all(running):
            messages.append(
                "Services not running that should be: {}"
                .format(", ".join(_filter_tuples(services_running, False))))
            state = 'blocked'
        # also verify that the ports that should be open are open
        # NB, that ServiceManager objects only OPTIONALLY have ports
        map_not_open, ports_open = (
            _check_listening_on_services_ports(services))
        if not all(ports_open):
            # find which service has missing ports. They are in service
            # order which makes it a bit easier.
            message_parts = {service: ", ".join([str(v) for v in open_ports])
                             for service, open_ports in map_not_open.items()}
            message = ", ".join(
                ["{}: [{}]".format(s, sp) for s, sp in message_parts.items()])
            messages.append(
                "Services with ports not open that should be: {}"
                .format(message))
            state = 'blocked'

    if ports is not None:
        # and we can also check ports which we don't know the service for
        ports_open, ports_open_bools = _check_listening_on_ports_list(ports)
        if not all(ports_open_bools):
            messages.append(
                "Ports which should be open, but are not: {}"
                .format(", ".join([str(p) for p, v in ports_open
                                   if not v])))
            state = 'blocked'

    if state is not None:
        message = "; ".join(messages)
        return state, message

    return None, None


def _extract_services_list_helper(services):
    """Extract a OrderedDict of {service: [ports]} of the supplied services
    for use by the other functions.

    The services object can either be:
      - None : no services were passed (an empty dict is returned)
      - a list of strings
      - A dictionary (optionally OrderedDict) {service_name: {'service': ..}}
      - An array of [{'service': service_name, ...}, ...]

    @param services: see above
    @returns OrderedDict(service: [ports], ...)
    """
    if services is None:
        return {}
    if isinstance(services, dict):
        services = services.values()
    # either extract the list of services from the dictionary, or if
    # it is a simple string, use that. i.e. works with mixed lists.
    _s = OrderedDict()
    for s in services:
        if isinstance(s, dict) and 'service' in s:
            _s[s['service']] = s.get('ports', [])
        if isinstance(s, str):
            _s[s] = []
    return _s


def _check_running_services(services):
    """Check that the services dict provided is actually running and provide
    a list of (service, boolean) tuples for each service.

    Returns both a zipped list of (service, boolean) and a list of booleans
    in the same order as the services.

    @param services: OrderedDict of strings: [ports], one for each service to
                     check.
    @returns [(service, boolean), ...], : results for checks
             [boolean]                  : just the result of the service checks
    """
    services_running = [service_running(s) for s in services]
    return list(zip(services, services_running)), services_running


def _check_listening_on_services_ports(services, test=False):
    """Check that the unit is actually listening (has the port open) on the
    ports that the service specifies are open. If test is True then the
    function returns the services with ports that are open rather than
    closed.

    Returns an OrderedDict of service: ports and a list of booleans

    @param services: OrderedDict(service: [port, ...], ...)
    @param test: default=False, if False, test for closed, otherwise open.
    @returns OrderedDict(service: [port-not-open, ...]...), [boolean]
    """
    test = not(not(test))  # ensure test is True or False
    all_ports = list(itertools.chain(*services.values()))
    ports_states = [port_has_listener('0.0.0.0', p) for p in all_ports]
    map_ports = OrderedDict()
    matched_ports = [p for p, opened in zip(all_ports, ports_states)
                     if opened == test]  # essentially opened xor test
    for service, ports in services.items():
        set_ports = set(ports).intersection(matched_ports)
        if set_ports:
            map_ports[service] = set_ports
    return map_ports, ports_states


def _check_listening_on_ports_list(ports):
    """Check that the ports list given are being listened to

    Returns a list of ports being listened to and a list of the
    booleans.

    @param ports: LIST or port numbers.
    @returns [(port_num, boolean), ...], [boolean]
    """
    ports_open = [port_has_listener('0.0.0.0', p) for p in ports]
    return zip(ports, ports_open), ports_open


def _filter_tuples(services_states, state):
    """Return a simple list from a list of tuples according to the condition

    @param services_states: LIST of (string, boolean): service and running
           state.
    @param state: Boolean to match the tuple against.
    @returns [LIST of strings] that matched the tuple RHS.
    """
    return [s for s, b in services_states if b == state]


def workload_state_compare(current_workload_state, workload_state):
    """ Return highest priority of two states"""
    hierarchy = {'unknown': -1,
                 'active': 0,
                 'maintenance': 1,
                 'waiting': 2,
                 'blocked': 3,
                 }

    if hierarchy.get(workload_state) is None:
        workload_state = 'unknown'
    if hierarchy.get(current_workload_state) is None:
        current_workload_state = 'unknown'

    # Set workload_state based on hierarchy of statuses
    if hierarchy.get(current_workload_state) > hierarchy.get(workload_state):
        return current_workload_state
    else:
        return workload_state


def incomplete_relation_data(configs, required_interfaces):
    """Check complete contexts against required_interfaces
    Return dictionary of incomplete relation data.

    configs is an OSConfigRenderer object with configs registered

    required_interfaces is a dictionary of required general interfaces
    with dictionary values of possible specific interfaces.
    Example:
    required_interfaces = {'database': ['shared-db', 'pgsql-db']}

    The interface is said to be satisfied if anyone of the interfaces in the
    list has a complete context.

    Return dictionary of incomplete or missing required contexts with relation
    status of interfaces and any missing data points. Example:
        {'message':
             {'amqp': {'missing_data': ['rabbitmq_password'], 'related': True},
              'zeromq-configuration': {'related': False}},
         'identity':
             {'identity-service': {'related': False}},
         'database':
             {'pgsql-db': {'related': False},
              'shared-db': {'related': True}}}
    """
    complete_ctxts = configs.complete_contexts()
    incomplete_relations = [
        svc_type
        for svc_type, interfaces in required_interfaces.items()
        if not set(interfaces).intersection(complete_ctxts)]
    return {
        i: configs.get_incomplete_context_data(required_interfaces[i])
        for i in incomplete_relations}


def do_action_openstack_upgrade(package, upgrade_callback, configs):
    """Perform action-managed OpenStack upgrade.

    Upgrades packages to the configured openstack-origin version and sets
    the corresponding action status as a result.

    If the charm was installed from source we cannot upgrade it.
    For backwards compatibility a config flag (action-managed-upgrade) must
    be set for this code to run, otherwise a full service level upgrade will
    fire on config-changed.

    @param package: package name for determining if upgrade available
    @param upgrade_callback: function callback to charm's upgrade function
    @param configs: templating object derived from OSConfigRenderer class

    @return: True if upgrade successful; False if upgrade failed or skipped
    """
    ret = False

    if openstack_upgrade_available(package):
        if config('action-managed-upgrade'):
            juju_log('Upgrading OpenStack release')

            try:
                upgrade_callback(configs=configs)
                action_set({'outcome': 'success, upgrade completed.'})
                ret = True
            except Exception:
                action_set({'outcome': 'upgrade failed, see traceback.'})
                action_set({'traceback': traceback.format_exc()})
                action_fail('do_openstack_upgrade resulted in an '
                            'unexpected error')
        else:
            action_set({'outcome': 'action-managed-upgrade config is '
                                   'False, skipped upgrade.'})
    else:
        action_set({'outcome': 'no upgrade available.'})

    return ret


def remote_restart(rel_name, remote_service=None):
    trigger = {
        'restart-trigger': str(uuid.uuid4()),
    }
    if remote_service:
        trigger['remote-service'] = remote_service
    for rid in relation_ids(rel_name):
        # This subordinate can be related to two seperate services using
        # different subordinate relations so only issue the restart if
        # the principle is conencted down the relation we think it is
        if related_units(relid=rid):
            relation_set(relation_id=rid,
                         relation_settings=trigger,
                         )


def check_actually_paused(services=None, ports=None):
    """Check that services listed in the services object and and ports
    are actually closed (not listened to), to verify that the unit is
    properly paused.

    @param services: See _extract_services_list_helper
    @returns status, : string for status (None if okay)
             message : string for problem for status_set
    """
    state = None
    message = None
    messages = []
    if services is not None:
        services = _extract_services_list_helper(services)
        services_running, services_states = _check_running_services(services)
        if any(services_states):
            # there shouldn't be any running so this is a problem
            messages.append("these services running: {}"
                            .format(", ".join(
                                _filter_tuples(services_running, True))))
            state = "blocked"
        ports_open, ports_open_bools = (
            _check_listening_on_services_ports(services, True))
        if any(ports_open_bools):
            message_parts = {service: ", ".join([str(v) for v in open_ports])
                             for service, open_ports in ports_open.items()}
            message = ", ".join(
                ["{}: [{}]".format(s, sp) for s, sp in message_parts.items()])
            messages.append(
                "these service:ports are open: {}".format(message))
            state = 'blocked'
    if ports is not None:
        ports_open, bools = _check_listening_on_ports_list(ports)
        if any(bools):
            messages.append(
                "these ports which should be closed, but are open: {}"
                .format(", ".join([str(p) for p, v in ports_open if v])))
            state = 'blocked'
    if messages:
        message = ("Services should be paused but {}"
                   .format(", ".join(messages)))
    return state, message


def set_unit_paused():
    """Set the unit to a paused state in the local kv() store.
    This does NOT actually pause the unit
    """
    with unitdata.HookData()() as t:
        kv = t[0]
        kv.set('unit-paused', True)


def clear_unit_paused():
    """Clear the unit from a paused state in the local kv() store
    This does NOT actually restart any services - it only clears the
    local state.
    """
    with unitdata.HookData()() as t:
        kv = t[0]
        kv.set('unit-paused', False)


def is_unit_paused_set():
    """Return the state of the kv().get('unit-paused').
    This does NOT verify that the unit really is paused.

    To help with units that don't have HookData() (testing)
    if it excepts, return False
    """
    try:
        with unitdata.HookData()() as t:
            kv = t[0]
            # transform something truth-y into a Boolean.
            return not(not(kv.get('unit-paused')))
    except Exception:
        return False


def pause_unit(assess_status_func, services=None, ports=None,
               charm_func=None):
    """Pause a unit by stopping the services and setting 'unit-paused'
    in the local kv() store.

    Also checks that the services have stopped and ports are no longer
    being listened to.

    An optional charm_func() can be called that can either raise an
    Exception or return non None, None to indicate that the unit
    didn't pause cleanly.

    The signature for charm_func is:
    charm_func() -> message: string

    charm_func() is executed after any services are stopped, if supplied.

    The services object can either be:
      - None : no services were passed (an empty dict is returned)
      - a list of strings
      - A dictionary (optionally OrderedDict) {service_name: {'service': ..}}
      - An array of [{'service': service_name, ...}, ...]

    @param assess_status_func: (f() -> message: string | None) or None
    @param services: OPTIONAL see above
    @param ports: OPTIONAL list of port
    @param charm_func: function to run for custom charm pausing.
    @returns None
    @raises Exception(message) on an error for action_fail().
    """
    services = _extract_services_list_helper(services)
    messages = []
    if services:
        for service in services.keys():
            stopped = service_pause(service)
            if not stopped:
                messages.append("{} didn't stop cleanly.".format(service))
    if charm_func:
        try:
            message = charm_func()
            if message:
                messages.append(message)
        except Exception as e:
            message.append(str(e))
    set_unit_paused()
    if assess_status_func:
        message = assess_status_func()
        if message:
            messages.append(message)
    if messages and not is_unit_upgrading_set():
        raise Exception("Couldn't pause: {}".format("; ".join(messages)))


def resume_unit(assess_status_func, services=None, ports=None,
                charm_func=None):
    """Resume a unit by starting the services and clearning 'unit-paused'
    in the local kv() store.

    Also checks that the services have started and ports are being listened to.

    An optional charm_func() can be called that can either raise an
    Exception or return non None to indicate that the unit
    didn't resume cleanly.

    The signature for charm_func is:
    charm_func() -> message: string

    charm_func() is executed after any services are started, if supplied.

    The services object can either be:
      - None : no services were passed (an empty dict is returned)
      - a list of strings
      - A dictionary (optionally OrderedDict) {service_name: {'service': ..}}
      - An array of [{'service': service_name, ...}, ...]

    @param assess_status_func: (f() -> message: string | None) or None
    @param services: OPTIONAL see above
    @param ports: OPTIONAL list of port
    @param charm_func: function to run for custom charm resuming.
    @returns None
    @raises Exception(message) on an error for action_fail().
    """
    services = _extract_services_list_helper(services)
    messages = []
    if services:
        for service in services.keys():
            started = service_resume(service)
            if not started:
                messages.append("{} didn't start cleanly.".format(service))
    if charm_func:
        try:
            message = charm_func()
            if message:
                messages.append(message)
        except Exception as e:
            message.append(str(e))
    clear_unit_paused()
    if assess_status_func:
        message = assess_status_func()
        if message:
            messages.append(message)
    if messages:
        raise Exception("Couldn't resume: {}".format("; ".join(messages)))


def make_assess_status_func(*args, **kwargs):
    """Creates an assess_status_func() suitable for handing to pause_unit()
    and resume_unit().

    This uses the _determine_os_workload_status(...) function to determine
    what the workload_status should be for the unit.  If the unit is
    not in maintenance or active states, then the message is returned to
    the caller.  This is so an action that doesn't result in either a
    complete pause or complete resume can signal failure with an action_fail()
    """
    def _assess_status_func():
        state, message = _determine_os_workload_status(*args, **kwargs)
        status_set(state, message)
        if state not in ['maintenance', 'active']:
            return message
        return None

    return _assess_status_func


def pausable_restart_on_change(restart_map, stopstart=False,
                               restart_functions=None):
    """A restart_on_change decorator that checks to see if the unit is
    paused. If it is paused then the decorated function doesn't fire.

    This is provided as a helper, as the @restart_on_change(...) decorator
    is in core.host, yet the openstack specific helpers are in this file
    (contrib.openstack.utils).  Thus, this needs to be an optional feature
    for openstack charms (or charms that wish to use the openstack
    pause/resume type features).

    It is used as follows:

        from contrib.openstack.utils import (
            pausable_restart_on_change as restart_on_change)

        @restart_on_change(restart_map, stopstart=<boolean>)
        def some_hook(...):
            pass

    see core.utils.restart_on_change() for more details.

    Note restart_map can be a callable, in which case, restart_map is only
    evaluated at runtime.  This means that it is lazy and the underlying
    function won't be called if the decorated function is never called.  Note,
    retains backwards compatibility for passing a non-callable dictionary.

    @param f: the function to decorate
    @param restart_map: (optionally callable, which then returns the
        restart_map) the restart map {conf_file: [services]}
    @param stopstart: DEFAULT false; whether to stop, start or just restart
    @returns decorator to use a restart_on_change with pausability
    """
    def wrap(f):
        # py27 compatible nonlocal variable.  When py3 only, replace with
        # nonlocal keyword
        __restart_map_cache = {'cache': None}

        @functools.wraps(f)
        def wrapped_f(*args, **kwargs):
            if is_unit_paused_set():
                return f(*args, **kwargs)
            if __restart_map_cache['cache'] is None:
                __restart_map_cache['cache'] = restart_map() \
                    if callable(restart_map) else restart_map
            # otherwise, normal restart_on_change functionality
            return restart_on_change_helper(
                (lambda: f(*args, **kwargs)), __restart_map_cache['cache'],
                stopstart, restart_functions)
        return wrapped_f
    return wrap


def ordered(orderme):
    """Converts the provided dictionary into a collections.OrderedDict.

    The items in the returned OrderedDict will be inserted based on the
    natural sort order of the keys. Nested dictionaries will also be sorted
    in order to ensure fully predictable ordering.

    :param orderme: the dict to order
    :return: collections.OrderedDict
    :raises: ValueError: if `orderme` isn't a dict instance.
    """
    if not isinstance(orderme, dict):
        raise ValueError('argument must be a dict type')

    result = OrderedDict()
    for k, v in sorted(six.iteritems(orderme), key=lambda x: x[0]):
        if isinstance(v, dict):
            result[k] = ordered(v)
        else:
            result[k] = v

    return result


def config_flags_parser(config_flags):
    """Parses config flags string into dict.

    This parsing method supports a few different formats for the config
    flag values to be parsed:

      1. A string in the simple format of key=value pairs, with the possibility
         of specifying multiple key value pairs within the same string. For
         example, a string in the format of 'key1=value1, key2=value2' will
         return a dict of:

             {'key1': 'value1', 'key2': 'value2'}.

      2. A string in the above format, but supporting a comma-delimited list
         of values for the same key. For example, a string in the format of
         'key1=value1, key2=value3,value4,value5' will return a dict of:

             {'key1': 'value1', 'key2': 'value2,value3,value4'}

      3. A string containing a colon character (:) prior to an equal
         character (=) will be treated as yaml and parsed as such. This can be
         used to specify more complex key value pairs. For example,
         a string in the format of 'key1: subkey1=value1, subkey2=value2' will
         return a dict of:

             {'key1', 'subkey1=value1, subkey2=value2'}

    The provided config_flags string may be a list of comma-separated values
    which themselves may be comma-separated list of values.
    """
    # If we find a colon before an equals sign then treat it as yaml.
    # Note: limit it to finding the colon first since this indicates assignment
    # for inline yaml.
    colon = config_flags.find(':')
    equals = config_flags.find('=')
    if colon > 0:
        if colon < equals or equals < 0:
            return ordered(yaml.safe_load(config_flags))

    if config_flags.find('==') >= 0:
        juju_log("config_flags is not in expected format (key=value)",
                 level=ERROR)
        raise OSContextError

    # strip the following from each value.
    post_strippers = ' ,'
    # we strip any leading/trailing '=' or ' ' from the string then
    # split on '='.
    split = config_flags.strip(' =').split('=')
    limit = len(split)
    flags = OrderedDict()
    for i in range(0, limit - 1):
        current = split[i]
        next = split[i + 1]
        vindex = next.rfind(',')
        if (i == limit - 2) or (vindex < 0):
            value = next
        else:
            value = next[:vindex]

        if i == 0:
            key = current
        else:
            # if this not the first entry, expect an embedded key.
            index = current.rfind(',')
            if index < 0:
                juju_log("Invalid config value(s) at index %s" % (i),
                         level=ERROR)
                raise OSContextError
            key = current[index + 1:]

        # Add to collection.
        flags[key.strip(post_strippers)] = value.rstrip(post_strippers)

    return flags


def os_application_version_set(package):
    '''Set version of application for Juju 2.0 and later'''
    application_version = get_upstream_version(package)
    # NOTE(jamespage) if not able to figure out package version, fallback to
    #                 openstack codename version detection.
    if not application_version:
        application_version_set(os_release(package))
    else:
        application_version_set(application_version)


def enable_memcache(source=None, release=None, package=None):
    """Determine if memcache should be enabled on the local unit

    @param release: release of OpenStack currently deployed
    @param package: package to derive OpenStack version deployed
    @returns boolean Whether memcache should be enabled
    """
    _release = None
    if release:
        _release = release
    else:
        _release = os_release(package, base='icehouse')
    if not _release:
        _release = get_os_codename_install_source(source)

    return CompareOpenStackReleases(_release) >= 'mitaka'


def token_cache_pkgs(source=None, release=None):
    """Determine additional packages needed for token caching

    @param source: source string for charm
    @param release: release of OpenStack currently deployed
    @returns List of package to enable token caching
    """
    packages = []
    if enable_memcache(source=source, release=release):
        packages.extend(['memcached', 'python-memcache'])
    return packages


def update_json_file(filename, items):
    """Updates the json `filename` with a given dict.
    :param filename: path to json file (e.g. /etc/glance/policy.json)
    :param items: dict of items to update
    """
    if not items:
        return

    with open(filename) as fd:
        policy = json.load(fd)

    # Compare before and after and if nothing has changed don't write the file
    # since that could cause unnecessary service restarts.
    before = json.dumps(policy, indent=4, sort_keys=True)
    policy.update(items)
    after = json.dumps(policy, indent=4, sort_keys=True)
    if before == after:
        return

    with open(filename, "w") as fd:
        fd.write(after)


@cached
def snap_install_requested():
    """ Determine if installing from snaps

    If openstack-origin is of the form snap:track/channel[/branch]
    and channel is in SNAPS_CHANNELS return True.
    """
    origin = config('openstack-origin') or ""
    if not origin.startswith('snap:'):
        return False

    _src = origin[5:]
    if '/' in _src:
        channel = _src.split('/')[1]
    else:
        # Handle snap:track with no channel
        channel = 'stable'
    return valid_snap_channel(channel)


def get_snaps_install_info_from_origin(snaps, src, mode='classic'):
    """Generate a dictionary of snap install information from origin

    @param snaps: List of snaps
    @param src: String of openstack-origin or source of the form
        snap:track/channel
    @param mode: String classic, devmode or jailmode
    @returns: Dictionary of snaps with channels and modes
    """

    if not src.startswith('snap:'):
        juju_log("Snap source is not a snap origin", 'WARN')
        return {}

    _src = src[5:]
    channel = '--channel={}'.format(_src)

    return {snap: {'channel': channel, 'mode': mode}
            for snap in snaps}


def install_os_snaps(snaps, refresh=False):
    """Install OpenStack snaps from channel and with mode

    @param snaps: Dictionary of snaps with channels and modes of the form:
        {'snap_name': {'channel': 'snap_channel',
                       'mode': 'snap_mode'}}
        Where channel is a snapstore channel and mode is --classic, --devmode
        or --jailmode.
    @param post_snap_install: Callback function to run after snaps have been
    installed
    """

    def _ensure_flag(flag):
        if flag.startswith('--'):
            return flag
        return '--{}'.format(flag)

    if refresh:
        for snap in snaps.keys():
            snap_refresh(snap,
                         _ensure_flag(snaps[snap]['channel']),
                         _ensure_flag(snaps[snap]['mode']))
    else:
        for snap in snaps.keys():
            snap_install(snap,
                         _ensure_flag(snaps[snap]['channel']),
                         _ensure_flag(snaps[snap]['mode']))


def set_unit_upgrading():
    """Set the unit to a upgrading state in the local kv() store.
    """
    with unitdata.HookData()() as t:
        kv = t[0]
        kv.set('unit-upgrading', True)


def clear_unit_upgrading():
    """Clear the unit from a upgrading state in the local kv() store
    """
    with unitdata.HookData()() as t:
        kv = t[0]
        kv.set('unit-upgrading', False)


def is_unit_upgrading_set():
    """Return the state of the kv().get('unit-upgrading').

    To help with units that don't have HookData() (testing)
    if it excepts, return False
    """
    try:
        with unitdata.HookData()() as t:
            kv = t[0]
            # transform something truth-y into a Boolean.
            return not(not(kv.get('unit-upgrading')))
    except Exception:
        return False


def series_upgrade_prepare(pause_unit_helper=None, configs=None):
    """ Run common series upgrade prepare tasks.

    :param pause_unit_helper: function: Function to pause unit
    :param configs: OSConfigRenderer object: Configurations
    :returns None:
    """
    set_unit_upgrading()
    if pause_unit_helper and configs:
        if not is_unit_paused_set():
            pause_unit_helper(configs)


def series_upgrade_complete(resume_unit_helper=None, configs=None):
    """ Run common series upgrade complete tasks.

    :param resume_unit_helper: function: Function to resume unit
    :param configs: OSConfigRenderer object: Configurations
    :returns None:
    """
    clear_unit_paused()
    clear_unit_upgrading()
    if configs:
        configs.write_all()
        if resume_unit_helper:
            resume_unit_helper(configs)
