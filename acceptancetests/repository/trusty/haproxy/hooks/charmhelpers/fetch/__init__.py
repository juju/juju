# Copyright 2014-2015 Canonical Limited.
#
# This file is part of charm-helpers.
#
# charm-helpers is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License version 3 as
# published by the Free Software Foundation.
#
# charm-helpers is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with charm-helpers.  If not, see <http://www.gnu.org/licenses/>.

import importlib
from tempfile import NamedTemporaryFile
import time
from yaml import safe_load
from charmhelpers.core.host import (
    lsb_release
)
import subprocess
from charmhelpers.core.hookenv import (
    config,
    log,
)
import os

import six
if six.PY3:
    from urllib.parse import urlparse, urlunparse
else:
    from urlparse import urlparse, urlunparse


CLOUD_ARCHIVE = """# Ubuntu Cloud Archive
deb http://ubuntu-cloud.archive.canonical.com/ubuntu {} main
"""
PROPOSED_POCKET = """# Proposed
deb http://archive.ubuntu.com/ubuntu {}-proposed main universe multiverse restricted
"""
CLOUD_ARCHIVE_POCKETS = {
    # Folsom
    'folsom': 'precise-updates/folsom',
    'precise-folsom': 'precise-updates/folsom',
    'precise-folsom/updates': 'precise-updates/folsom',
    'precise-updates/folsom': 'precise-updates/folsom',
    'folsom/proposed': 'precise-proposed/folsom',
    'precise-folsom/proposed': 'precise-proposed/folsom',
    'precise-proposed/folsom': 'precise-proposed/folsom',
    # Grizzly
    'grizzly': 'precise-updates/grizzly',
    'precise-grizzly': 'precise-updates/grizzly',
    'precise-grizzly/updates': 'precise-updates/grizzly',
    'precise-updates/grizzly': 'precise-updates/grizzly',
    'grizzly/proposed': 'precise-proposed/grizzly',
    'precise-grizzly/proposed': 'precise-proposed/grizzly',
    'precise-proposed/grizzly': 'precise-proposed/grizzly',
    # Havana
    'havana': 'precise-updates/havana',
    'precise-havana': 'precise-updates/havana',
    'precise-havana/updates': 'precise-updates/havana',
    'precise-updates/havana': 'precise-updates/havana',
    'havana/proposed': 'precise-proposed/havana',
    'precise-havana/proposed': 'precise-proposed/havana',
    'precise-proposed/havana': 'precise-proposed/havana',
    # Icehouse
    'icehouse': 'precise-updates/icehouse',
    'precise-icehouse': 'precise-updates/icehouse',
    'precise-icehouse/updates': 'precise-updates/icehouse',
    'precise-updates/icehouse': 'precise-updates/icehouse',
    'icehouse/proposed': 'precise-proposed/icehouse',
    'precise-icehouse/proposed': 'precise-proposed/icehouse',
    'precise-proposed/icehouse': 'precise-proposed/icehouse',
    # Juno
    'juno': 'trusty-updates/juno',
    'trusty-juno': 'trusty-updates/juno',
    'trusty-juno/updates': 'trusty-updates/juno',
    'trusty-updates/juno': 'trusty-updates/juno',
    'juno/proposed': 'trusty-proposed/juno',
    'trusty-juno/proposed': 'trusty-proposed/juno',
    'trusty-proposed/juno': 'trusty-proposed/juno',
    # Kilo
    'kilo': 'trusty-updates/kilo',
    'trusty-kilo': 'trusty-updates/kilo',
    'trusty-kilo/updates': 'trusty-updates/kilo',
    'trusty-updates/kilo': 'trusty-updates/kilo',
    'kilo/proposed': 'trusty-proposed/kilo',
    'trusty-kilo/proposed': 'trusty-proposed/kilo',
    'trusty-proposed/kilo': 'trusty-proposed/kilo',
    # Liberty
    'liberty': 'trusty-updates/liberty',
    'trusty-liberty': 'trusty-updates/liberty',
    'trusty-liberty/updates': 'trusty-updates/liberty',
    'trusty-updates/liberty': 'trusty-updates/liberty',
    'liberty/proposed': 'trusty-proposed/liberty',
    'trusty-liberty/proposed': 'trusty-proposed/liberty',
    'trusty-proposed/liberty': 'trusty-proposed/liberty',
}

# The order of this list is very important. Handlers should be listed in from
# least- to most-specific URL matching.
FETCH_HANDLERS = (
    'charmhelpers.fetch.archiveurl.ArchiveUrlFetchHandler',
    'charmhelpers.fetch.bzrurl.BzrUrlFetchHandler',
    'charmhelpers.fetch.giturl.GitUrlFetchHandler',
)

APT_NO_LOCK = 100  # The return code for "couldn't acquire lock" in APT.
APT_NO_LOCK_RETRY_DELAY = 10  # Wait 10 seconds between apt lock checks.
APT_NO_LOCK_RETRY_COUNT = 30  # Retry to acquire the lock X times.


class SourceConfigError(Exception):
    pass


class UnhandledSource(Exception):
    pass


class AptLockError(Exception):
    pass


class BaseFetchHandler(object):

    """Base class for FetchHandler implementations in fetch plugins"""

    def can_handle(self, source):
        """Returns True if the source can be handled. Otherwise returns
        a string explaining why it cannot"""
        return "Wrong source type"

    def install(self, source):
        """Try to download and unpack the source. Return the path to the
        unpacked files or raise UnhandledSource."""
        raise UnhandledSource("Wrong source type {}".format(source))

    def parse_url(self, url):
        return urlparse(url)

    def base_url(self, url):
        """Return url without querystring or fragment"""
        parts = list(self.parse_url(url))
        parts[4:] = ['' for i in parts[4:]]
        return urlunparse(parts)


def filter_installed_packages(packages):
    """Returns a list of packages that require installation"""
    cache = apt_cache()
    _pkgs = []
    for package in packages:
        try:
            p = cache[package]
            p.current_ver or _pkgs.append(package)
        except KeyError:
            log('Package {} has no installation candidate.'.format(package),
                level='WARNING')
            _pkgs.append(package)
    return _pkgs


def apt_cache(in_memory=True):
    """Build and return an apt cache"""
    from apt import apt_pkg
    apt_pkg.init()
    if in_memory:
        apt_pkg.config.set("Dir::Cache::pkgcache", "")
        apt_pkg.config.set("Dir::Cache::srcpkgcache", "")
    return apt_pkg.Cache()


def apt_install(packages, options=None, fatal=False):
    """Install one or more packages"""
    if options is None:
        options = ['--option=Dpkg::Options::=--force-confold']

    cmd = ['apt-get', '--assume-yes']
    cmd.extend(options)
    cmd.append('install')
    if isinstance(packages, six.string_types):
        cmd.append(packages)
    else:
        cmd.extend(packages)
    log("Installing {} with options: {}".format(packages,
                                                options))
    _run_apt_command(cmd, fatal)


def apt_upgrade(options=None, fatal=False, dist=False):
    """Upgrade all packages"""
    if options is None:
        options = ['--option=Dpkg::Options::=--force-confold']

    cmd = ['apt-get', '--assume-yes']
    cmd.extend(options)
    if dist:
        cmd.append('dist-upgrade')
    else:
        cmd.append('upgrade')
    log("Upgrading with options: {}".format(options))
    _run_apt_command(cmd, fatal)


def apt_update(fatal=False):
    """Update local apt cache"""
    cmd = ['apt-get', 'update']
    _run_apt_command(cmd, fatal)


def apt_purge(packages, fatal=False):
    """Purge one or more packages"""
    cmd = ['apt-get', '--assume-yes', 'purge']
    if isinstance(packages, six.string_types):
        cmd.append(packages)
    else:
        cmd.extend(packages)
    log("Purging {}".format(packages))
    _run_apt_command(cmd, fatal)


def apt_hold(packages, fatal=False):
    """Hold one or more packages"""
    cmd = ['apt-mark', 'hold']
    if isinstance(packages, six.string_types):
        cmd.append(packages)
    else:
        cmd.extend(packages)
    log("Holding {}".format(packages))

    if fatal:
        subprocess.check_call(cmd)
    else:
        subprocess.call(cmd)


def add_source(source, key=None):
    """Add a package source to this system.

    @param source: a URL or sources.list entry, as supported by
    add-apt-repository(1). Examples::

        ppa:charmers/example
        deb https://stub:key@private.example.com/ubuntu trusty main

    In addition:
        'proposed:' may be used to enable the standard 'proposed'
        pocket for the release.
        'cloud:' may be used to activate official cloud archive pockets,
        such as 'cloud:icehouse'
        'distro' may be used as a noop

    @param key: A key to be added to the system's APT keyring and used
    to verify the signatures on packages. Ideally, this should be an
    ASCII format GPG public key including the block headers. A GPG key
    id may also be used, but be aware that only insecure protocols are
    available to retrieve the actual public key from a public keyserver
    placing your Juju environment at risk. ppa and cloud archive keys
    are securely added automtically, so sould not be provided.
    """
    if source is None:
        log('Source is not present. Skipping')
        return

    if (source.startswith('ppa:') or
        source.startswith('http') or
        source.startswith('deb ') or
            source.startswith('cloud-archive:')):
        subprocess.check_call(['add-apt-repository', '--yes', source])
    elif source.startswith('cloud:'):
        apt_install(filter_installed_packages(['ubuntu-cloud-keyring']),
                    fatal=True)
        pocket = source.split(':')[-1]
        if pocket not in CLOUD_ARCHIVE_POCKETS:
            raise SourceConfigError(
                'Unsupported cloud: source option %s' %
                pocket)
        actual_pocket = CLOUD_ARCHIVE_POCKETS[pocket]
        with open('/etc/apt/sources.list.d/cloud-archive.list', 'w') as apt:
            apt.write(CLOUD_ARCHIVE.format(actual_pocket))
    elif source == 'proposed':
        release = lsb_release()['DISTRIB_CODENAME']
        with open('/etc/apt/sources.list.d/proposed.list', 'w') as apt:
            apt.write(PROPOSED_POCKET.format(release))
    elif source == 'distro':
        pass
    else:
        log("Unknown source: {!r}".format(source))

    if key:
        if '-----BEGIN PGP PUBLIC KEY BLOCK-----' in key:
            with NamedTemporaryFile('w+') as key_file:
                key_file.write(key)
                key_file.flush()
                key_file.seek(0)
                subprocess.check_call(['apt-key', 'add', '-'], stdin=key_file)
        else:
            # Note that hkp: is in no way a secure protocol. Using a
            # GPG key id is pointless from a security POV unless you
            # absolutely trust your network and DNS.
            subprocess.check_call(['apt-key', 'adv', '--keyserver',
                                   'hkp://keyserver.ubuntu.com:80', '--recv',
                                   key])


def configure_sources(update=False,
                      sources_var='install_sources',
                      keys_var='install_keys'):
    """
    Configure multiple sources from charm configuration.

    The lists are encoded as yaml fragments in the configuration.
    The frament needs to be included as a string. Sources and their
    corresponding keys are of the types supported by add_source().

    Example config:
        install_sources: |
          - "ppa:foo"
          - "http://example.com/repo precise main"
        install_keys: |
          - null
          - "a1b2c3d4"

    Note that 'null' (a.k.a. None) should not be quoted.
    """
    sources = safe_load((config(sources_var) or '').strip()) or []
    keys = safe_load((config(keys_var) or '').strip()) or None

    if isinstance(sources, six.string_types):
        sources = [sources]

    if keys is None:
        for source in sources:
            add_source(source, None)
    else:
        if isinstance(keys, six.string_types):
            keys = [keys]

        if len(sources) != len(keys):
            raise SourceConfigError(
                'Install sources and keys lists are different lengths')
        for source, key in zip(sources, keys):
            add_source(source, key)
    if update:
        apt_update(fatal=True)


def install_remote(source, *args, **kwargs):
    """
    Install a file tree from a remote source

    The specified source should be a url of the form:
        scheme://[host]/path[#[option=value][&...]]

    Schemes supported are based on this modules submodules.
    Options supported are submodule-specific.
    Additional arguments are passed through to the submodule.

    For example::

        dest = install_remote('http://example.com/archive.tgz',
                              checksum='deadbeef',
                              hash_type='sha1')

    This will download `archive.tgz`, validate it using SHA1 and, if
    the file is ok, extract it and return the directory in which it
    was extracted.  If the checksum fails, it will raise
    :class:`charmhelpers.core.host.ChecksumError`.
    """
    # We ONLY check for True here because can_handle may return a string
    # explaining why it can't handle a given source.
    handlers = [h for h in plugins() if h.can_handle(source) is True]
    installed_to = None
    for handler in handlers:
        try:
            installed_to = handler.install(source, *args, **kwargs)
        except UnhandledSource:
            pass
    if not installed_to:
        raise UnhandledSource("No handler found for source {}".format(source))
    return installed_to


def install_from_config(config_var_name):
    charm_config = config()
    source = charm_config[config_var_name]
    return install_remote(source)


def plugins(fetch_handlers=None):
    if not fetch_handlers:
        fetch_handlers = FETCH_HANDLERS
    plugin_list = []
    for handler_name in fetch_handlers:
        package, classname = handler_name.rsplit('.', 1)
        try:
            handler_class = getattr(
                importlib.import_module(package),
                classname)
            plugin_list.append(handler_class())
        except (ImportError, AttributeError):
            # Skip missing plugins so that they can be ommitted from
            # installation if desired
            log("FetchHandler {} not found, skipping plugin".format(
                handler_name))
    return plugin_list


def _run_apt_command(cmd, fatal=False):
    """
    Run an APT command, checking output and retrying if the fatal flag is set
    to True.

    :param: cmd: str: The apt command to run.
    :param: fatal: bool: Whether the command's output should be checked and
        retried.
    """
    env = os.environ.copy()

    if 'DEBIAN_FRONTEND' not in env:
        env['DEBIAN_FRONTEND'] = 'noninteractive'

    if fatal:
        retry_count = 0
        result = None

        # If the command is considered "fatal", we need to retry if the apt
        # lock was not acquired.

        while result is None or result == APT_NO_LOCK:
            try:
                result = subprocess.check_call(cmd, env=env)
            except subprocess.CalledProcessError as e:
                retry_count = retry_count + 1
                if retry_count > APT_NO_LOCK_RETRY_COUNT:
                    raise
                result = e.returncode
                log("Couldn't acquire DPKG lock. Will retry in {} seconds."
                    "".format(APT_NO_LOCK_RETRY_DELAY))
                time.sleep(APT_NO_LOCK_RETRY_DELAY)

    else:
        subprocess.call(cmd, env=env)
