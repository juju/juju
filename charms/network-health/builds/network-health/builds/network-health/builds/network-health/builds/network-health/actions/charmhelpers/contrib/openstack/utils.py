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
import shutil

import six
import tempfile
import traceback
import uuid
import yaml

from charmhelpers.contrib.network import ip

from charmhelpers.core import (
    unitdata,
)

from charmhelpers.core.hookenv import (
    action_fail,
    action_set,
    config,
    log as juju_log,
    charm_dir,
    DEBUG,
    INFO,
    ERROR,
    related_units,
    relation_ids,
    relation_set,
    service_name,
    status_set,
    hook_name,
    application_version_set,
)

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

from charmhelpers.contrib.python.packages import (
    pip_create_virtualenv,
    pip_install,
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
    apt_install,
    apt_cache,
    install_remote,
    get_upstream_version
)
from charmhelpers.contrib.storage.linux.utils import is_block_device, zap_disk
from charmhelpers.contrib.storage.linux.loopback import ensure_loopback_device
from charmhelpers.contrib.openstack.exceptions import OSContextError

CLOUD_ARCHIVE_URL = "http://ubuntu-cloud.archive.canonical.com/ubuntu"
CLOUD_ARCHIVE_KEY_ID = '5EDB1B62EC4926EA'

DISTRO_PROPOSED = ('deb http://archive.ubuntu.com/ubuntu/ %s-proposed '
                   'restricted main multiverse universe')

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
        ['2.11.0']),
])

# >= Liberty version->codename mapping
PACKAGE_CODENAMES = {
    'nova-common': OrderedDict([
        ('12', 'liberty'),
        ('13', 'mitaka'),
        ('14', 'newton'),
        ('15', 'ocata'),
    ]),
    'neutron-common': OrderedDict([
        ('7', 'liberty'),
        ('8', 'mitaka'),
        ('9', 'newton'),
        ('10', 'ocata'),
    ]),
    'cinder-common': OrderedDict([
        ('7', 'liberty'),
        ('8', 'mitaka'),
        ('9', 'newton'),
        ('10', 'ocata'),
    ]),
    'keystone': OrderedDict([
        ('8', 'liberty'),
        ('9', 'mitaka'),
        ('10', 'newton'),
        ('11', 'ocata'),
    ]),
    'horizon-common': OrderedDict([
        ('8', 'liberty'),
        ('9', 'mitaka'),
        ('10', 'newton'),
        ('11', 'ocata'),
    ]),
    'ceilometer-common': OrderedDict([
        ('5', 'liberty'),
        ('6', 'mitaka'),
        ('7', 'newton'),
        ('8', 'ocata'),
    ]),
    'heat-common': OrderedDict([
        ('5', 'liberty'),
        ('6', 'mitaka'),
        ('7', 'newton'),
        ('8', 'ocata'),
    ]),
    'glance-common': OrderedDict([
        ('11', 'liberty'),
        ('12', 'mitaka'),
        ('13', 'newton'),
        ('14', 'ocata'),
    ]),
    'openstack-dashboard': OrderedDict([
        ('8', 'liberty'),
        ('9', 'mitaka'),
        ('10', 'newton'),
        ('11', 'ocata'),
    ]),
}

GIT_DEFAULT_REPOS = {
    'requirements': 'git://github.com/openstack/requirements',
    'cinder': 'git://github.com/openstack/cinder',
    'glance': 'git://github.com/openstack/glance',
    'horizon': 'git://github.com/openstack/horizon',
    'keystone': 'git://github.com/openstack/keystone',
    'networking-hyperv': 'git://github.com/openstack/networking-hyperv',
    'neutron': 'git://github.com/openstack/neutron',
    'neutron-fwaas': 'git://github.com/openstack/neutron-fwaas',
    'neutron-lbaas': 'git://github.com/openstack/neutron-lbaas',
    'neutron-vpnaas': 'git://github.com/openstack/neutron-vpnaas',
    'nova': 'git://github.com/openstack/nova',
}

GIT_DEFAULT_BRANCHES = {
    'liberty': 'stable/liberty',
    'mitaka': 'stable/mitaka',
    'newton': 'stable/newton',
    'master': 'master',
}

DEFAULT_LOOPBACK_SIZE = '5G'


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
        ca_rel = ca_rel.split('%s-' % ubuntu_rel)[1].split('/')[0]
        return ca_rel

    # Best guess match based on deb string provided
    if src.startswith('deb') or src.startswith('ppa'):
        for k, v in six.iteritems(OPENSTACK_CODENAMES):
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
            if codename in ret or release[0] in ret:
                return codename
    elif len(codenames) == 1:
        return codenames[0]

    # NOTE: fallback - attempt to match with just major.minor version
    match = re.match('^(\d+)\.(\d+)', version)
    if match:
        major_minor_version = match.group(0)
        for codename, versions in six.iteritems(SWIFT_CODENAMES):
            for release_version in versions:
                if release_version.startswith(major_minor_version):
                    return codename

    return None


def get_os_codename_package(package, fatal=True):
    '''Derive OpenStack release codename from an installed package.'''
    import apt_pkg as apt

    cache = apt_cache()

    try:
        pkg = cache[package]
    except:
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
        match = re.match('^(\d+)\.(\d+)\.(\d+)', vers)
    else:
        # x.y match only for 20XX.X
        # and ignore patch level for other packages
        match = re.match('^(\d+)\.(\d+)', vers)

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


os_rel = None


def reset_os_release():
    '''Unset the cached os_release version'''
    global os_rel
    os_rel = None


def os_release(package, base='essex', reset_cache=False):
    '''
    Returns OpenStack release codename from a cached global.

    If reset_cache then unset the cached os_release version and return the
    freshly determined version.

    If the codename can not be determined from either an installed package or
    the installation source, the earliest release supported by the charm should
    be returned.
    '''
    global os_rel
    if reset_cache:
        reset_os_release()
    if os_rel:
        return os_rel
    os_rel = (git_os_codename_install_source(config('openstack-origin-git')) or
              get_os_codename_package(package, fatal=False) or
              get_os_codename_install_source(config('openstack-origin')) or
              base)
    return os_rel


def import_key(keyid):
    key = keyid.strip()
    if (key.startswith('-----BEGIN PGP PUBLIC KEY BLOCK-----') and
            key.endswith('-----END PGP PUBLIC KEY BLOCK-----')):
        juju_log("PGP key found (looks like ASCII Armor format)", level=DEBUG)
        juju_log("Importing ASCII Armor PGP key", level=DEBUG)
        with tempfile.NamedTemporaryFile() as keyfile:
            with open(keyfile.name, 'w') as fd:
                fd.write(key)
                fd.write("\n")

            cmd = ['apt-key', 'add', keyfile.name]
            try:
                subprocess.check_call(cmd)
            except subprocess.CalledProcessError:
                error_out("Error importing PGP key '%s'" % key)
    else:
        juju_log("PGP key found (looks like Radix64 format)", level=DEBUG)
        juju_log("Importing PGP key from keyserver", level=DEBUG)
        cmd = ['apt-key', 'adv', '--keyserver',
               'hkp://keyserver.ubuntu.com:80', '--recv-keys', key]
        try:
            subprocess.check_call(cmd)
        except subprocess.CalledProcessError:
            error_out("Error importing PGP key '%s'" % key)


def get_source_and_pgp_key(input):
    """Look for a pgp key ID or ascii-armor key in the given input."""
    index = input.strip()
    index = input.rfind('|')
    if index < 0:
        return input, None

    key = input[index + 1:].strip('|')
    source = input[:index]
    return source, key


def configure_installation_source(rel):
    '''Configure apt installation source.'''
    if rel == 'distro':
        return
    elif rel == 'distro-proposed':
        ubuntu_rel = lsb_release()['DISTRIB_CODENAME']
        with open('/etc/apt/sources.list.d/juju_deb.list', 'w') as f:
            f.write(DISTRO_PROPOSED % ubuntu_rel)
    elif rel[:4] == "ppa:":
        src, key = get_source_and_pgp_key(rel)
        if key:
            import_key(key)

        subprocess.check_call(["add-apt-repository", "-y", src])
    elif rel[:3] == "deb":
        src, key = get_source_and_pgp_key(rel)
        if key:
            import_key(key)

        with open('/etc/apt/sources.list.d/juju_deb.list', 'w') as f:
            f.write(src)
    elif rel[:6] == 'cloud:':
        ubuntu_rel = lsb_release()['DISTRIB_CODENAME']
        rel = rel.split(':')[1]
        u_rel = rel.split('-')[0]
        ca_rel = rel.split('-')[1]

        if u_rel != ubuntu_rel:
            e = 'Cannot install from Cloud Archive pocket %s on this Ubuntu '\
                'version (%s)' % (ca_rel, ubuntu_rel)
            error_out(e)

        if 'staging' in ca_rel:
            # staging is just a regular PPA.
            os_rel = ca_rel.split('/')[0]
            ppa = 'ppa:ubuntu-cloud-archive/%s-staging' % os_rel
            cmd = 'add-apt-repository -y %s' % ppa
            subprocess.check_call(cmd.split(' '))
            return

        # map charm config options to actual archive pockets.
        pockets = {
            'folsom': 'precise-updates/folsom',
            'folsom/updates': 'precise-updates/folsom',
            'folsom/proposed': 'precise-proposed/folsom',
            'grizzly': 'precise-updates/grizzly',
            'grizzly/updates': 'precise-updates/grizzly',
            'grizzly/proposed': 'precise-proposed/grizzly',
            'havana': 'precise-updates/havana',
            'havana/updates': 'precise-updates/havana',
            'havana/proposed': 'precise-proposed/havana',
            'icehouse': 'precise-updates/icehouse',
            'icehouse/updates': 'precise-updates/icehouse',
            'icehouse/proposed': 'precise-proposed/icehouse',
            'juno': 'trusty-updates/juno',
            'juno/updates': 'trusty-updates/juno',
            'juno/proposed': 'trusty-proposed/juno',
            'kilo': 'trusty-updates/kilo',
            'kilo/updates': 'trusty-updates/kilo',
            'kilo/proposed': 'trusty-proposed/kilo',
            'liberty': 'trusty-updates/liberty',
            'liberty/updates': 'trusty-updates/liberty',
            'liberty/proposed': 'trusty-proposed/liberty',
            'mitaka': 'trusty-updates/mitaka',
            'mitaka/updates': 'trusty-updates/mitaka',
            'mitaka/proposed': 'trusty-proposed/mitaka',
            'newton': 'xenial-updates/newton',
            'newton/updates': 'xenial-updates/newton',
            'newton/proposed': 'xenial-proposed/newton',
            'ocata': 'xenial-updates/ocata',
            'ocata/updates': 'xenial-updates/ocata',
            'ocata/proposed': 'xenial-proposed/ocata',
        }

        try:
            pocket = pockets[ca_rel]
        except KeyError:
            e = 'Invalid Cloud Archive release specified: %s' % rel
            error_out(e)

        src = "deb %s %s main" % (CLOUD_ARCHIVE_URL, pocket)
        apt_install('ubuntu-cloud-keyring', fatal=True)

        with open('/etc/apt/sources.list.d/cloud-archive.list', 'w') as f:
            f.write(src)
    else:
        error_out("Invalid openstack-release specified: %s" % rel)


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
    with open(juju_rc_path, 'wb') as rc_script:
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
    if "swift" in package:
        codename = get_os_codename_install_source(src)
        avail_vers = get_os_version_codename_swift(codename)
    else:
        avail_vers = get_os_version_install_source(src)
    apt.init()
    if "swift" in package:
        major_cur_vers = cur_vers.split('.', 1)[0]
        major_avail_vers = avail_vers.split('.', 1)[0]
        major_diff = apt.version_compare(major_avail_vers, major_cur_vers)
        return avail_vers > cur_vers and (major_diff == 1 or major_diff == 0)
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


def git_install_requested():
    """
    Returns true if openstack-origin-git is specified.
    """
    return config('openstack-origin-git') is not None


def git_os_codename_install_source(projects_yaml):
    """
    Returns OpenStack codename of release being installed from source.
    """
    if git_install_requested():
        projects = _git_yaml_load(projects_yaml)

        if projects in GIT_DEFAULT_BRANCHES.keys():
            if projects == 'master':
                return 'ocata'
            return projects

        if 'release' in projects:
            if projects['release'] == 'master':
                return 'ocata'
            return projects['release']

    return None


def git_default_repos(projects_yaml):
    """
    Returns default repos if a default openstack-origin-git value is specified.
    """
    service = service_name()
    core_project = service

    for default, branch in GIT_DEFAULT_BRANCHES.iteritems():
        if projects_yaml == default:

            # add the requirements repo first
            repo = {
                'name': 'requirements',
                'repository': GIT_DEFAULT_REPOS['requirements'],
                'branch': branch,
            }
            repos = [repo]

            # neutron-* and nova-* charms require some additional repos
            if service in ['neutron-api', 'neutron-gateway',
                           'neutron-openvswitch']:
                core_project = 'neutron'
                if service == 'neutron-api':
                    repo = {
                        'name': 'networking-hyperv',
                        'repository': GIT_DEFAULT_REPOS['networking-hyperv'],
                        'branch': branch,
                    }
                    repos.append(repo)
                for project in ['neutron-fwaas', 'neutron-lbaas',
                                'neutron-vpnaas', 'nova']:
                    repo = {
                        'name': project,
                        'repository': GIT_DEFAULT_REPOS[project],
                        'branch': branch,
                    }
                    repos.append(repo)

            elif service in ['nova-cloud-controller', 'nova-compute']:
                core_project = 'nova'
                repo = {
                    'name': 'neutron',
                    'repository': GIT_DEFAULT_REPOS['neutron'],
                    'branch': branch,
                }
                repos.append(repo)
            elif service == 'openstack-dashboard':
                core_project = 'horizon'

            # finally add the current service's core project repo
            repo = {
                'name': core_project,
                'repository': GIT_DEFAULT_REPOS[core_project],
                'branch': branch,
            }
            repos.append(repo)

            return yaml.dump(dict(repositories=repos, release=default))

    return projects_yaml


def _git_yaml_load(projects_yaml):
    """
    Load the specified yaml into a dictionary.
    """
    if not projects_yaml:
        return None

    return yaml.load(projects_yaml)


requirements_dir = None


def git_clone_and_install(projects_yaml, core_project):
    """
    Clone/install all specified OpenStack repositories.

    The expected format of projects_yaml is:

        repositories:
          - {name: keystone,
             repository: 'git://git.openstack.org/openstack/keystone.git',
             branch: 'stable/icehouse'}
          - {name: requirements,
             repository: 'git://git.openstack.org/openstack/requirements.git',
             branch: 'stable/icehouse'}

        directory: /mnt/openstack-git
        http_proxy: squid-proxy-url
        https_proxy: squid-proxy-url

    The directory, http_proxy, and https_proxy keys are optional.

    """
    global requirements_dir
    parent_dir = '/mnt/openstack-git'
    http_proxy = None

    projects = _git_yaml_load(projects_yaml)
    _git_validate_projects_yaml(projects, core_project)

    old_environ = dict(os.environ)

    if 'http_proxy' in projects.keys():
        http_proxy = projects['http_proxy']
        os.environ['http_proxy'] = projects['http_proxy']
    if 'https_proxy' in projects.keys():
        os.environ['https_proxy'] = projects['https_proxy']

    if 'directory' in projects.keys():
        parent_dir = projects['directory']

    pip_create_virtualenv(os.path.join(parent_dir, 'venv'))

    # Upgrade setuptools and pip from default virtualenv versions. The default
    # versions in trusty break master OpenStack branch deployments.
    for p in ['pip', 'setuptools']:
        pip_install(p, upgrade=True, proxy=http_proxy,
                    venv=os.path.join(parent_dir, 'venv'))

    constraints = None
    for p in projects['repositories']:
        repo = p['repository']
        branch = p['branch']
        depth = '1'
        if 'depth' in p.keys():
            depth = p['depth']
        if p['name'] == 'requirements':
            repo_dir = _git_clone_and_install_single(repo, branch, depth,
                                                     parent_dir, http_proxy,
                                                     update_requirements=False)
            requirements_dir = repo_dir
            constraints = os.path.join(repo_dir, "upper-constraints.txt")
            # upper-constraints didn't exist until after icehouse
            if not os.path.isfile(constraints):
                constraints = None
            # use constraints unless project yaml sets use_constraints to false
            if 'use_constraints' in projects.keys():
                if not projects['use_constraints']:
                    constraints = None
        else:
            repo_dir = _git_clone_and_install_single(repo, branch, depth,
                                                     parent_dir, http_proxy,
                                                     update_requirements=True,
                                                     constraints=constraints)

    os.environ = old_environ


def _git_validate_projects_yaml(projects, core_project):
    """
    Validate the projects yaml.
    """
    _git_ensure_key_exists('repositories', projects)

    for project in projects['repositories']:
        _git_ensure_key_exists('name', project.keys())
        _git_ensure_key_exists('repository', project.keys())
        _git_ensure_key_exists('branch', project.keys())

    if projects['repositories'][0]['name'] != 'requirements':
        error_out('{} git repo must be specified first'.format('requirements'))

    if projects['repositories'][-1]['name'] != core_project:
        error_out('{} git repo must be specified last'.format(core_project))

    _git_ensure_key_exists('release', projects)


def _git_ensure_key_exists(key, keys):
    """
    Ensure that key exists in keys.
    """
    if key not in keys:
        error_out('openstack-origin-git key \'{}\' is missing'.format(key))


def _git_clone_and_install_single(repo, branch, depth, parent_dir, http_proxy,
                                  update_requirements, constraints=None):
    """
    Clone and install a single git repository.
    """
    if not os.path.exists(parent_dir):
        juju_log('Directory already exists at {}. '
                 'No need to create directory.'.format(parent_dir))
        os.mkdir(parent_dir)

    juju_log('Cloning git repo: {}, branch: {}'.format(repo, branch))
    repo_dir = install_remote(
        repo, dest=parent_dir, branch=branch, depth=depth)

    venv = os.path.join(parent_dir, 'venv')

    if update_requirements:
        if not requirements_dir:
            error_out('requirements repo must be cloned before '
                      'updating from global requirements.')
        _git_update_requirements(venv, repo_dir, requirements_dir)

    juju_log('Installing git repo from dir: {}'.format(repo_dir))
    if http_proxy:
        pip_install(repo_dir, proxy=http_proxy, venv=venv,
                    constraints=constraints)
    else:
        pip_install(repo_dir, venv=venv, constraints=constraints)

    return repo_dir


def _git_update_requirements(venv, package_dir, reqs_dir):
    """
    Update from global requirements.

    Update an OpenStack git directory's requirements.txt and
    test-requirements.txt from global-requirements.txt.
    """
    orig_dir = os.getcwd()
    os.chdir(reqs_dir)
    python = os.path.join(venv, 'bin/python')
    cmd = [python, 'update.py', package_dir]
    try:
        subprocess.check_call(cmd)
    except subprocess.CalledProcessError:
        package = os.path.basename(package_dir)
        error_out("Error updating {} from "
                  "global-requirements.txt".format(package))
    os.chdir(orig_dir)


def git_pip_venv_dir(projects_yaml):
    """
    Return the pip virtualenv path.
    """
    parent_dir = '/mnt/openstack-git'

    projects = _git_yaml_load(projects_yaml)

    if 'directory' in projects.keys():
        parent_dir = projects['directory']

    return os.path.join(parent_dir, 'venv')


def git_src_dir(projects_yaml, project):
    """
    Return the directory where the specified project's source is located.
    """
    parent_dir = '/mnt/openstack-git'

    projects = _git_yaml_load(projects_yaml)

    if 'directory' in projects.keys():
        parent_dir = projects['directory']

    for p in projects['repositories']:
        if p['name'] == project:
            return os.path.join(parent_dir, os.path.basename(p['repository']))

    return None


def git_yaml_value(projects_yaml, key):
    """
    Return the value in projects_yaml for the specified key.
    """
    projects = _git_yaml_load(projects_yaml)

    if key in projects.keys():
        return projects[key]

    return None


def git_generate_systemd_init_files(templates_dir):
    """
    Generate systemd init files.

    Generates and installs systemd init units and script files based on the
    *.init.in files contained in the templates_dir directory.

    This code is based on the openstack-pkg-tools package and its init
    script generation, which is used by the OpenStack packages.
    """
    for f in os.listdir(templates_dir):
        # Create the init script and systemd unit file from the template
        if f.endswith(".init.in"):
            init_in_file = f
            init_file = f[:-8]
            service_file = "{}.service".format(init_file)

            init_in_source = os.path.join(templates_dir, init_in_file)
            init_source = os.path.join(templates_dir, init_file)
            service_source = os.path.join(templates_dir, service_file)

            init_dest = os.path.join('/etc/init.d', init_file)
            service_dest = os.path.join('/lib/systemd/system', service_file)

            shutil.copyfile(init_in_source, init_source)
            with open(init_source, 'a') as outfile:
                template = '/usr/share/openstack-pkg-tools/init-script-template'
                with open(template) as infile:
                    outfile.write('\n\n{}'.format(infile.read()))

            cmd = ['pkgos-gen-systemd-unit', init_in_source]
            subprocess.check_call(cmd)

            if os.path.exists(init_dest):
                os.remove(init_dest)
            if os.path.exists(service_dest):
                os.remove(service_dest)
            shutil.copyfile(init_source, init_dest)
            shutil.copyfile(service_source, service_dest)
            os.chmod(init_dest, 0o755)

    for f in os.listdir(templates_dir):
        # If there's a service.in file, use it instead of the generated one
        if f.endswith(".service.in"):
            service_in_file = f
            service_file = f[:-3]

            service_in_source = os.path.join(templates_dir, service_in_file)
            service_source = os.path.join(templates_dir, service_file)
            service_dest = os.path.join('/lib/systemd/system', service_file)

            shutil.copyfile(service_in_source, service_source)

            if os.path.exists(service_dest):
                os.remove(service_dest)
            shutil.copyfile(service_source, service_dest)

    for f in os.listdir(templates_dir):
        # Generate the systemd unit if there's no existing .service.in
        if f.endswith(".init.in"):
            init_in_file = f
            init_file = f[:-8]
            service_in_file = "{}.service.in".format(init_file)
            service_file = "{}.service".format(init_file)

            init_in_source = os.path.join(templates_dir, init_in_file)
            service_in_source = os.path.join(templates_dir, service_in_file)
            service_source = os.path.join(templates_dir, service_file)
            service_dest = os.path.join('/lib/systemd/system', service_file)

            if not os.path.exists(service_in_source):
                cmd = ['pkgos-gen-systemd-unit', init_in_source]
                subprocess.check_call(cmd)

                if os.path.exists(service_dest):
                    os.remove(service_dest)
                shutil.copyfile(service_source, service_dest)


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

    if the unit isn't supposed to be paused, just return None, None

    @param services: OPTIONAL services spec or list of service names.
    @param ports: OPTIONAL list of port numbers.
    @returns state, message or None, None
    """
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

    if git_install_requested():
        action_set({'outcome': 'installed from source, skipped upgrade.'})
    else:
        if openstack_upgrade_available(package):
            if config('action-managed-upgrade'):
                juju_log('Upgrading OpenStack release')

                try:
                    upgrade_callback(configs=configs)
                    action_set({'outcome': 'success, upgrade completed.'})
                    ret = True
                except:
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
    except:
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
    if messages:
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

    @param f: the function to decorate
    @param restart_map: the restart map {conf_file: [services]}
    @param stopstart: DEFAULT false; whether to stop, start or just restart
    @returns decorator to use a restart_on_change with pausability
    """
    def wrap(f):
        @functools.wraps(f)
        def wrapped_f(*args, **kwargs):
            if is_unit_paused_set():
                return f(*args, **kwargs)
            # otherwise, normal restart_on_change functionality
            return restart_on_change_helper(
                (lambda: f(*args, **kwargs)), restart_map, stopstart,
                restart_functions)
        return wrapped_f
    return wrap


def config_flags_parser(config_flags):
    """Parses config flags string into dict.

    This parsing method supports a few different formats for the config
    flag values to be parsed:

      1. A string in the simple format of key=value pairs, with the possibility
         of specifying multiple key value pairs within the same string. For
         example, a string in the format of 'key1=value1, key2=value2' will
         return a dict of:

             {'key1': 'value1',
              'key2': 'value2'}.

      2. A string in the above format, but supporting a comma-delimited list
         of values for the same key. For example, a string in the format of
         'key1=value1, key2=value3,value4,value5' will return a dict of:

             {'key1', 'value1',
              'key2', 'value2,value3,value4'}

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
            return yaml.safe_load(config_flags)

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
    flags = {}
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


def enable_memcache(source=None, release=None):
    """Determine if memcache should be enabled on the local unit

    @param source: source string for charm
    @param release: release of OpenStack currently deployed
    @returns boolean Whether memcache should be enabled
    """
    if not release:
        release = get_os_codename_install_source(source)
    return release >= 'mitaka'


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
