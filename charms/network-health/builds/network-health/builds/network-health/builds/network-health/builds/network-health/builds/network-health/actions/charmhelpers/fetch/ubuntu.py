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

import os
import six
import time
import subprocess

from tempfile import NamedTemporaryFile
from charmhelpers.core.host import (
    lsb_release
)
from charmhelpers.core.hookenv import log
from charmhelpers.fetch import SourceConfigError

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
    # Mitaka
    'mitaka': 'trusty-updates/mitaka',
    'trusty-mitaka': 'trusty-updates/mitaka',
    'trusty-mitaka/updates': 'trusty-updates/mitaka',
    'trusty-updates/mitaka': 'trusty-updates/mitaka',
    'mitaka/proposed': 'trusty-proposed/mitaka',
    'trusty-mitaka/proposed': 'trusty-proposed/mitaka',
    'trusty-proposed/mitaka': 'trusty-proposed/mitaka',
    # Newton
    'newton': 'xenial-updates/newton',
    'xenial-newton': 'xenial-updates/newton',
    'xenial-newton/updates': 'xenial-updates/newton',
    'xenial-updates/newton': 'xenial-updates/newton',
    'newton/proposed': 'xenial-proposed/newton',
    'xenial-newton/proposed': 'xenial-proposed/newton',
    'xenial-proposed/newton': 'xenial-proposed/newton',
    # Ocata
    'ocata': 'xenial-updates/ocata',
    'xenial-ocata': 'xenial-updates/ocata',
    'xenial-ocata/updates': 'xenial-updates/ocata',
    'xenial-updates/ocata': 'xenial-updates/ocata',
    'ocata/proposed': 'xenial-proposed/ocata',
    'xenial-ocata/proposed': 'xenial-proposed/ocata',
    'xenial-ocata/newton': 'xenial-proposed/ocata',
}

APT_NO_LOCK = 100  # The return code for "couldn't acquire lock" in APT.
APT_NO_LOCK_RETRY_DELAY = 10  # Wait 10 seconds between apt lock checks.
APT_NO_LOCK_RETRY_COUNT = 30  # Retry to acquire the lock X times.


def filter_installed_packages(packages):
    """Return a list of packages that require installation."""
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


def apt_cache(in_memory=True, progress=None):
    """Build and return an apt cache."""
    from apt import apt_pkg
    apt_pkg.init()
    if in_memory:
        apt_pkg.config.set("Dir::Cache::pkgcache", "")
        apt_pkg.config.set("Dir::Cache::srcpkgcache", "")
    return apt_pkg.Cache(progress)


def install(packages, options=None, fatal=False):
    """Install one or more packages."""
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


def upgrade(options=None, fatal=False, dist=False):
    """Upgrade all packages."""
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


def update(fatal=False):
    """Update local apt cache."""
    cmd = ['apt-get', 'update']
    _run_apt_command(cmd, fatal)


def purge(packages, fatal=False):
    """Purge one or more packages."""
    cmd = ['apt-get', '--assume-yes', 'purge']
    if isinstance(packages, six.string_types):
        cmd.append(packages)
    else:
        cmd.extend(packages)
    log("Purging {}".format(packages))
    _run_apt_command(cmd, fatal)


def apt_mark(packages, mark, fatal=False):
    """Flag one or more packages using apt-mark."""
    log("Marking {} as {}".format(packages, mark))
    cmd = ['apt-mark', mark]
    if isinstance(packages, six.string_types):
        cmd.append(packages)
    else:
        cmd.extend(packages)

    if fatal:
        subprocess.check_call(cmd, universal_newlines=True)
    else:
        subprocess.call(cmd, universal_newlines=True)


def apt_hold(packages, fatal=False):
    return apt_mark(packages, 'hold', fatal=fatal)


def apt_unhold(packages, fatal=False):
    return apt_mark(packages, 'unhold', fatal=fatal)


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
        install(filter_installed_packages(['ubuntu-cloud-keyring']),
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


def _run_apt_command(cmd, fatal=False):
    """Run an APT command.

    Checks the output and retries if the fatal flag is set
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


def get_upstream_version(package):
    """Determine upstream version based on installed package

    @returns None (if not installed) or the upstream version
    """
    import apt_pkg
    cache = apt_cache()
    try:
        pkg = cache[package]
    except:
        # the package is unknown to the current apt cache.
        return None

    if not pkg.current_ver:
        # package is known, but no version is currently installed.
        return None

    return apt_pkg.upstream_version(pkg.current_ver.ver_str)
