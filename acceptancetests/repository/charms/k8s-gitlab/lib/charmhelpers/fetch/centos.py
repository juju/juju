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

import subprocess
import os
import time
import six
import yum

from tempfile import NamedTemporaryFile
from charmhelpers.core.hookenv import log

YUM_NO_LOCK = 1  # The return code for "couldn't acquire lock" in YUM.
YUM_NO_LOCK_RETRY_DELAY = 10  # Wait 10 seconds between apt lock checks.
YUM_NO_LOCK_RETRY_COUNT = 30  # Retry to acquire the lock X times.


def filter_installed_packages(packages):
    """Return a list of packages that require installation."""
    yb = yum.YumBase()
    package_list = yb.doPackageLists()
    temp_cache = {p.base_package_name: 1 for p in package_list['installed']}

    _pkgs = [p for p in packages if not temp_cache.get(p, False)]
    return _pkgs


def install(packages, options=None, fatal=False):
    """Install one or more packages."""
    cmd = ['yum', '--assumeyes']
    if options is not None:
        cmd.extend(options)
    cmd.append('install')
    if isinstance(packages, six.string_types):
        cmd.append(packages)
    else:
        cmd.extend(packages)
    log("Installing {} with options: {}".format(packages,
                                                options))
    _run_yum_command(cmd, fatal)


def upgrade(options=None, fatal=False, dist=False):
    """Upgrade all packages."""
    cmd = ['yum', '--assumeyes']
    if options is not None:
        cmd.extend(options)
    cmd.append('upgrade')
    log("Upgrading with options: {}".format(options))
    _run_yum_command(cmd, fatal)


def update(fatal=False):
    """Update local yum cache."""
    cmd = ['yum', '--assumeyes', 'update']
    log("Update with fatal: {}".format(fatal))
    _run_yum_command(cmd, fatal)


def purge(packages, fatal=False):
    """Purge one or more packages."""
    cmd = ['yum', '--assumeyes', 'remove']
    if isinstance(packages, six.string_types):
        cmd.append(packages)
    else:
        cmd.extend(packages)
    log("Purging {}".format(packages))
    _run_yum_command(cmd, fatal)


def yum_search(packages):
    """Search for a package."""
    output = {}
    cmd = ['yum', 'search']
    if isinstance(packages, six.string_types):
        cmd.append(packages)
    else:
        cmd.extend(packages)
    log("Searching for {}".format(packages))
    result = subprocess.check_output(cmd)
    for package in list(packages):
        output[package] = package in result
    return output


def add_source(source, key=None):
    """Add a package source to this system.

    @param source: a URL with a rpm package

    @param key: A key to be added to the system's keyring and used
    to verify the signatures on packages. Ideally, this should be an
    ASCII format GPG public key including the block headers. A GPG key
    id may also be used, but be aware that only insecure protocols are
    available to retrieve the actual public key from a public keyserver
    placing your Juju environment at risk.
    """
    if source is None:
        log('Source is not present. Skipping')
        return

    if source.startswith('http'):
        directory = '/etc/yum.repos.d/'
        for filename in os.listdir(directory):
            with open(directory + filename, 'r') as rpm_file:
                if source in rpm_file.read():
                    break
        else:
            log("Add source: {!r}".format(source))
            # write in the charms.repo
            with open(directory + 'Charms.repo', 'a') as rpm_file:
                rpm_file.write('[%s]\n' % source[7:].replace('/', '_'))
                rpm_file.write('name=%s\n' % source[7:])
                rpm_file.write('baseurl=%s\n\n' % source)
    else:
        log("Unknown source: {!r}".format(source))

    if key:
        if '-----BEGIN PGP PUBLIC KEY BLOCK-----' in key:
            with NamedTemporaryFile('w+') as key_file:
                key_file.write(key)
                key_file.flush()
                key_file.seek(0)
                subprocess.check_call(['rpm', '--import', key_file.name])
        else:
            subprocess.check_call(['rpm', '--import', key])


def _run_yum_command(cmd, fatal=False):
    """Run an YUM command.

    Checks the output and retry if the fatal flag is set to True.

    :param: cmd: str: The yum command to run.
    :param: fatal: bool: Whether the command's output should be checked and
        retried.
    """
    env = os.environ.copy()

    if fatal:
        retry_count = 0
        result = None

        # If the command is considered "fatal", we need to retry if the yum
        # lock was not acquired.

        while result is None or result == YUM_NO_LOCK:
            try:
                result = subprocess.check_call(cmd, env=env)
            except subprocess.CalledProcessError as e:
                retry_count = retry_count + 1
                if retry_count > YUM_NO_LOCK_RETRY_COUNT:
                    raise
                result = e.returncode
                log("Couldn't acquire YUM lock. Will retry in {} seconds."
                    "".format(YUM_NO_LOCK_RETRY_DELAY))
                time.sleep(YUM_NO_LOCK_RETRY_DELAY)

    else:
        subprocess.call(cmd, env=env)
