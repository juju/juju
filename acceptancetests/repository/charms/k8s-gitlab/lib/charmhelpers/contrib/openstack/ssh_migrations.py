# Copyright 2018 Canonical Ltd
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
import subprocess

from charmhelpers.core.hookenv import (
    ERROR,
    log,
    relation_get,
)
from charmhelpers.contrib.network.ip import (
    is_ipv6,
    ns_query,
)
from charmhelpers.contrib.openstack.utils import (
    get_hostname,
    get_host_ip,
    is_ip,
)

NOVA_SSH_DIR = '/etc/nova/compute_ssh/'


def ssh_directory_for_unit(application_name, user=None):
    """Return the directory used to store ssh assets for the application.

    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    :returns: Fully qualified directory path.
    :rtype: str
    """
    if user:
        application_name = "{}_{}".format(application_name, user)
    _dir = os.path.join(NOVA_SSH_DIR, application_name)
    for d in [NOVA_SSH_DIR, _dir]:
        if not os.path.isdir(d):
            os.mkdir(d)
    for f in ['authorized_keys', 'known_hosts']:
        f = os.path.join(_dir, f)
        if not os.path.isfile(f):
            open(f, 'w').close()
    return _dir


def known_hosts(application_name, user=None):
    """Return the known hosts file for the application.

    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    :returns: Fully qualified path to file.
    :rtype: str
    """
    return os.path.join(
        ssh_directory_for_unit(application_name, user),
        'known_hosts')


def authorized_keys(application_name, user=None):
    """Return the authorized keys file for the application.

    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    :returns: Fully qualified path to file.
    :rtype: str
    """
    return os.path.join(
        ssh_directory_for_unit(application_name, user),
        'authorized_keys')


def ssh_known_host_key(host, application_name, user=None):
    """Return the first entry in known_hosts for host.

    :param host: hostname to lookup in file.
    :type host: str
    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    :returns: Host key
    :rtype: str or None
    """
    cmd = [
        'ssh-keygen',
        '-f', known_hosts(application_name, user),
        '-H',
        '-F',
        host]
    try:
        # The first line of output is like '# Host xx found: line 1 type RSA',
        # which should be excluded.
        output = subprocess.check_output(cmd)
    except subprocess.CalledProcessError as e:
        # RC of 1 seems to be legitimate for most ssh-keygen -F calls.
        if e.returncode == 1:
            output = e.output
        else:
            raise
    output = output.strip()

    if output:
        # Bug #1500589 cmd has 0 rc on precise if entry not present
        lines = output.split('\n')
        if len(lines) >= 1:
            return lines[0]

    return None


def remove_known_host(host, application_name, user=None):
    """Remove the entry in known_hosts for host.

    :param host: hostname to lookup in file.
    :type host: str
    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    """
    log('Removing SSH known host entry for compute host at %s' % host)
    cmd = ['ssh-keygen', '-f', known_hosts(application_name, user), '-R', host]
    subprocess.check_call(cmd)


def is_same_key(key_1, key_2):
    """Extract the key from two host entries and compare them.

    :param key_1: Host key
    :type key_1: str
    :param key_2: Host key
    :type key_2: str
    """
    # The key format get will be like '|1|2rUumCavEXWVaVyB5uMl6m85pZo=|Cp'
    # 'EL6l7VTY37T/fg/ihhNb/GPgs= ssh-rsa AAAAB', we only need to compare
    # the part start with 'ssh-rsa' followed with '= ', because the hash
    # value in the beginning will change each time.
    k_1 = key_1.split('= ')[1]
    k_2 = key_2.split('= ')[1]
    return k_1 == k_2


def add_known_host(host, application_name, user=None):
    """Add the given host key to the known hosts file.

    :param host: host name
    :type host: str
    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    """
    cmd = ['ssh-keyscan', '-H', '-t', 'rsa', host]
    try:
        remote_key = subprocess.check_output(cmd).strip()
    except Exception as e:
        log('Could not obtain SSH host key from %s' % host, level=ERROR)
        raise e

    current_key = ssh_known_host_key(host, application_name, user)
    if current_key and remote_key:
        if is_same_key(remote_key, current_key):
            log('Known host key for compute host %s up to date.' % host)
            return
        else:
            remove_known_host(host, application_name, user)

    log('Adding SSH host key to known hosts for compute node at %s.' % host)
    with open(known_hosts(application_name, user), 'a') as out:
        out.write("{}\n".format(remote_key))


def ssh_authorized_key_exists(public_key, application_name, user=None):
    """Check if given key is in the authorized_key file.

    :param public_key: Public key.
    :type public_key: str
    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    :returns: Whether given key is in the authorized_key file.
    :rtype: boolean
    """
    with open(authorized_keys(application_name, user)) as keys:
        return ('%s' % public_key) in keys.read()


def add_authorized_key(public_key, application_name, user=None):
    """Add given key to the authorized_key file.

    :param public_key: Public key.
    :type public_key: str
    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    """
    with open(authorized_keys(application_name, user), 'a') as keys:
        keys.write("{}\n".format(public_key))


def ssh_compute_add_host_and_key(public_key, hostname, private_address,
                                 application_name, user=None):
    """Add a compute nodes ssh details to local cache.

    Collect various hostname variations and add the corresponding host keys to
    the local known hosts file. Finally, add the supplied public key to the
    authorized_key file.

    :param public_key: Public key.
    :type public_key: str
    :param hostname: Hostname to collect host keys from.
    :type hostname: str
    :param private_address:aCorresponding private address for hostname
    :type private_address: str
    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    """
    # If remote compute node hands us a hostname, ensure we have a
    # known hosts entry for its IP, hostname and FQDN.
    hosts = [private_address]

    if not is_ipv6(private_address):
        if hostname:
            hosts.append(hostname)

        if is_ip(private_address):
            hn = get_hostname(private_address)
            if hn:
                hosts.append(hn)
                short = hn.split('.')[0]
                if ns_query(short):
                    hosts.append(short)
        else:
            hosts.append(get_host_ip(private_address))
            short = private_address.split('.')[0]
            if ns_query(short):
                hosts.append(short)

    for host in list(set(hosts)):
        add_known_host(host, application_name, user)

    if not ssh_authorized_key_exists(public_key, application_name, user):
        log('Saving SSH authorized key for compute host at %s.' %
            private_address)
        add_authorized_key(public_key, application_name, user)


def ssh_compute_add(public_key, application_name, rid=None, unit=None,
                    user=None):
    """Add a compute nodes ssh details to local cache.

    Collect various hostname variations and add the corresponding host keys to
    the local known hosts file. Finally, add the supplied public key to the
    authorized_key file.

    :param public_key: Public key.
    :type public_key: str
    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param rid: Relation id of the relation between this charm and the app. If
                none is supplied it is assumed its the relation relating to
                the current hook context.
    :type rid: str
    :param unit: Unit to add ssh asserts for if none is supplied it is assumed
                 its the unit relating to the current hook context.
    :type unit: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    """
    relation_data = relation_get(rid=rid, unit=unit)
    ssh_compute_add_host_and_key(
        public_key,
        relation_data.get('hostname'),
        relation_data.get('private-address'),
        application_name,
        user=user)


def ssh_known_hosts_lines(application_name, user=None):
    """Return contents of known_hosts file for given application.

    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    """
    known_hosts_list = []
    with open(known_hosts(application_name, user)) as hosts:
        for hosts_line in hosts:
            if hosts_line.rstrip():
                known_hosts_list.append(hosts_line.rstrip())
    return(known_hosts_list)


def ssh_authorized_keys_lines(application_name, user=None):
    """Return contents of authorized_keys file for given application.

    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    """
    authorized_keys_list = []

    with open(authorized_keys(application_name, user)) as keys:
        for authkey_line in keys:
            if authkey_line.rstrip():
                authorized_keys_list.append(authkey_line.rstrip())
    return(authorized_keys_list)


def ssh_compute_remove(public_key, application_name, user=None):
    """Remove given public key from authorized_keys file.

    :param public_key: Public key.
    :type public_key: str
    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    """
    if not (os.path.isfile(authorized_keys(application_name, user)) or
            os.path.isfile(known_hosts(application_name, user))):
        return

    keys = ssh_authorized_keys_lines(application_name, user=None)
    keys = [k.strip() for k in keys]

    if public_key not in keys:
        return

    [keys.remove(key) for key in keys if key == public_key]

    with open(authorized_keys(application_name, user), 'w') as _keys:
        keys = '\n'.join(keys)
        if not keys.endswith('\n'):
            keys += '\n'
        _keys.write(keys)


def get_ssh_settings(application_name, user=None):
    """Retrieve the known host entries and public keys for application

    Retrieve the known host entries and public keys for application for all
    units of the given application related to this application for the
    app + user combination.

    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :param user: The user that the ssh asserts are for.
    :type user: str
    :returns: Public keys + host keys for all units for app + user combination.
    :rtype: dict
    """
    settings = {}
    keys = {}
    prefix = ''
    if user:
        prefix = '{}_'.format(user)

    for i, line in enumerate(ssh_known_hosts_lines(
            application_name=application_name, user=user)):
        settings['{}known_hosts_{}'.format(prefix, i)] = line
    if settings:
        settings['{}known_hosts_max_index'.format(prefix)] = len(
            settings.keys())

    for i, line in enumerate(ssh_authorized_keys_lines(
            application_name=application_name, user=user)):
        keys['{}authorized_keys_{}'.format(prefix, i)] = line
    if keys:
        keys['{}authorized_keys_max_index'.format(prefix)] = len(keys.keys())
    settings.update(keys)
    return settings


def get_all_user_ssh_settings(application_name):
    """Retrieve the known host entries and public keys for application

    Retrieve the known host entries and public keys for application for all
    units of the given application related to this application for root user
    and nova user.

    :param application_name: Name of application eg nova-compute-something
    :type application_name: str
    :returns: Public keys + host keys for all units for app + user combination.
    :rtype: dict
    """
    settings = get_ssh_settings(application_name)
    settings.update(get_ssh_settings(application_name, user='nova'))
    return settings
