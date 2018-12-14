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

# Easy file synchronization among peer units using ssh + unison.
#
# For the -joined, -changed, and -departed peer relations, add a call to
# ssh_authorized_peers() describing the peer relation and the desired
# user + group.  After all peer relations have settled, all hosts should
# be able to connect to on another via key auth'd ssh as the specified user.
#
# Other hooks are then free to synchronize files and directories using
# sync_to_peers().
#
# For a peer relation named 'cluster', for example:
#
# cluster-relation-joined:
# ...
# ssh_authorized_peers(peer_interface='cluster',
#                      user='juju_ssh', group='juju_ssh',
#                      ensure_local_user=True)
# ...
#
# cluster-relation-changed:
# ...
# ssh_authorized_peers(peer_interface='cluster',
#                      user='juju_ssh', group='juju_ssh',
#                      ensure_local_user=True)
# ...
#
# cluster-relation-departed:
# ...
# ssh_authorized_peers(peer_interface='cluster',
#                      user='juju_ssh', group='juju_ssh',
#                      ensure_local_user=True)
# ...
#
# Hooks are now free to sync files as easily as:
#
# files = ['/etc/fstab', '/etc/apt.conf.d/']
# sync_to_peers(peer_interface='cluster',
#                user='juju_ssh, paths=[files])
#
# It is assumed the charm itself has setup permissions on each unit
# such that 'juju_ssh' has read + write permissions.  Also assumed
# that the calling charm takes care of leader delegation.
#
# Additionally files can be synchronized only to an specific unit:
# sync_to_peer(slave_address, user='juju_ssh',
#              paths=[files], verbose=False)

import os
import pwd

from copy import copy
from subprocess import check_call, check_output

from charmhelpers.core.host import (
    adduser,
    add_user_to_group,
    pwgen,
    remove_password_expiry,
)

from charmhelpers.core.hookenv import (
    log,
    hook_name,
    relation_ids,
    related_units,
    relation_set,
    relation_get,
    unit_private_ip,
    INFO,
    ERROR,
)

BASE_CMD = ['unison', '-auto', '-batch=true', '-confirmbigdel=false',
            '-fastcheck=true', '-group=false', '-owner=false',
            '-prefer=newer', '-times=true']


def get_homedir(user):
    try:
        user = pwd.getpwnam(user)
        return user.pw_dir
    except KeyError:
        log('Could not get homedir for user %s: user exists?' % (user), ERROR)
        raise Exception


def create_private_key(user, priv_key_path, key_type='rsa'):
    types_bits = {
        'rsa': '2048',
        'ecdsa': '521',
    }
    if key_type not in types_bits:
        log('Unknown ssh key type {}, using rsa'.format(key_type), ERROR)
        key_type = 'rsa'
    if not os.path.isfile(priv_key_path):
        log('Generating new SSH key for user %s.' % user)
        cmd = ['ssh-keygen', '-q', '-N', '', '-t', key_type,
               '-b', types_bits[key_type], '-f', priv_key_path]
        check_call(cmd)
    else:
        log('SSH key already exists at %s.' % priv_key_path)
    check_call(['chown', user, priv_key_path])
    check_call(['chmod', '0600', priv_key_path])


def create_public_key(user, priv_key_path, pub_key_path):
    if not os.path.isfile(pub_key_path):
        log('Generating missing ssh public key @ %s.' % pub_key_path)
        cmd = ['ssh-keygen', '-y', '-f', priv_key_path]
        p = check_output(cmd).strip()
        with open(pub_key_path, 'wb') as out:
            out.write(p)
    check_call(['chown', user, pub_key_path])


def get_keypair(user):
    home_dir = get_homedir(user)
    ssh_dir = os.path.join(home_dir, '.ssh')
    priv_key = os.path.join(ssh_dir, 'id_rsa')
    pub_key = '%s.pub' % priv_key

    if not os.path.isdir(ssh_dir):
        os.mkdir(ssh_dir)
        check_call(['chown', '-R', user, ssh_dir])

    create_private_key(user, priv_key)
    create_public_key(user, priv_key, pub_key)

    with open(priv_key, 'r') as p:
        _priv = p.read().strip()

    with open(pub_key, 'r') as p:
        _pub = p.read().strip()

    return (_priv, _pub)


def write_authorized_keys(user, keys):
    home_dir = get_homedir(user)
    ssh_dir = os.path.join(home_dir, '.ssh')
    auth_keys = os.path.join(ssh_dir, 'authorized_keys')
    log('Syncing authorized_keys @ %s.' % auth_keys)
    with open(auth_keys, 'w') as out:
        for k in keys:
            out.write('%s\n' % k)


def write_known_hosts(user, hosts):
    home_dir = get_homedir(user)
    ssh_dir = os.path.join(home_dir, '.ssh')
    known_hosts = os.path.join(ssh_dir, 'known_hosts')
    khosts = []
    for host in hosts:
        cmd = ['ssh-keyscan', host]
        remote_key = check_output(cmd, universal_newlines=True).strip()
        khosts.append(remote_key)
    log('Syncing known_hosts @ %s.' % known_hosts)
    with open(known_hosts, 'w') as out:
        for host in khosts:
            out.write('%s\n' % host)


def ensure_user(user, group=None):
    adduser(user, pwgen())
    if group:
        add_user_to_group(user, group)
    # Remove password expiry (Bug #1686085)
    remove_password_expiry(user)


def ssh_authorized_peers(peer_interface, user, group=None,
                         ensure_local_user=False):
    """
    Main setup function, should be called from both peer -changed and -joined
    hooks with the same parameters.
    """
    if ensure_local_user:
        ensure_user(user, group)
    priv_key, pub_key = get_keypair(user)
    hook = hook_name()
    if hook == '%s-relation-joined' % peer_interface:
        relation_set(ssh_pub_key=pub_key)
    elif hook == '%s-relation-changed' % peer_interface or \
            hook == '%s-relation-departed' % peer_interface:
        hosts = []
        keys = []

        for r_id in relation_ids(peer_interface):
            for unit in related_units(r_id):
                ssh_pub_key = relation_get('ssh_pub_key',
                                           rid=r_id,
                                           unit=unit)
                priv_addr = relation_get('private-address',
                                         rid=r_id,
                                         unit=unit)
                if ssh_pub_key:
                    keys.append(ssh_pub_key)
                    hosts.append(priv_addr)
                else:
                    log('ssh_authorized_peers(): ssh_pub_key '
                        'missing for unit %s, skipping.' % unit)
        write_authorized_keys(user, keys)
        write_known_hosts(user, hosts)
        authed_hosts = ':'.join(hosts)
        relation_set(ssh_authorized_hosts=authed_hosts)


def _run_as_user(user, gid=None):
    try:
        user = pwd.getpwnam(user)
    except KeyError:
        log('Invalid user: %s' % user)
        raise Exception
    uid = user.pw_uid
    gid = gid or user.pw_gid
    os.environ['HOME'] = user.pw_dir

    def _inner():
        os.setgid(gid)
        os.setuid(uid)
    return _inner


def run_as_user(user, cmd, gid=None):
    return check_output(cmd, preexec_fn=_run_as_user(user, gid), cwd='/')


def collect_authed_hosts(peer_interface):
    '''Iterate through the units on peer interface to find all that
    have the calling host in its authorized hosts list'''
    hosts = []
    for r_id in (relation_ids(peer_interface) or []):
        for unit in related_units(r_id):
            private_addr = relation_get('private-address',
                                        rid=r_id, unit=unit)
            authed_hosts = relation_get('ssh_authorized_hosts',
                                        rid=r_id, unit=unit)

            if not authed_hosts:
                log('Peer %s has not authorized *any* hosts yet, skipping.' %
                    (unit), level=INFO)
                continue

            if unit_private_ip() in authed_hosts.split(':'):
                hosts.append(private_addr)
            else:
                log('Peer %s has not authorized *this* host yet, skipping.' %
                    (unit), level=INFO)
    return hosts


def sync_path_to_host(path, host, user, verbose=False, cmd=None, gid=None,
                      fatal=False):
    """Sync path to an specific peer host

    Propagates exception if operation fails and fatal=True.
    """
    cmd = cmd or copy(BASE_CMD)
    if not verbose:
        cmd.append('-silent')

    # removing trailing slash from directory paths, unison
    # doesn't like these.
    if path.endswith('/'):
        path = path[:(len(path) - 1)]

    cmd = cmd + [path, 'ssh://%s@%s/%s' % (user, host, path)]

    try:
        log('Syncing local path %s to %s@%s:%s' % (path, user, host, path))
        run_as_user(user, cmd, gid)
    except Exception:
        log('Error syncing remote files')
        if fatal:
            raise


def sync_to_peer(host, user, paths=None, verbose=False, cmd=None, gid=None,
                 fatal=False):
    """Sync paths to an specific peer host

    Propagates exception if any operation fails and fatal=True.
    """
    if paths:
        for p in paths:
            sync_path_to_host(p, host, user, verbose, cmd, gid, fatal)


def sync_to_peers(peer_interface, user, paths=None, verbose=False, cmd=None,
                  gid=None, fatal=False):
    """Sync all hosts to an specific path

    The type of group is integer, it allows user has permissions to
    operate a directory have a different group id with the user id.

    Propagates exception if any operation fails and fatal=True.
    """
    if paths:
        for host in collect_authed_hosts(peer_interface):
            sync_to_peer(host, user, paths, verbose, cmd, gid, fatal)
