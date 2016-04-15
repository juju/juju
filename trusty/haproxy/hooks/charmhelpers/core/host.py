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

"""Tools for working with the host system"""
# Copyright 2012 Canonical Ltd.
#
# Authors:
#  Nick Moffitt <nick.moffitt@canonical.com>
#  Matthew Wedgwood <matthew.wedgwood@canonical.com>

import os
import re
import pwd
import grp
import random
import string
import subprocess
import hashlib
from contextlib import contextmanager
from collections import OrderedDict

import six

from .hookenv import log
from .fstab import Fstab


def service_start(service_name):
    """Start a system service"""
    return service('start', service_name)


def service_stop(service_name):
    """Stop a system service"""
    return service('stop', service_name)


def service_restart(service_name):
    """Restart a system service"""
    return service('restart', service_name)


def service_reload(service_name, restart_on_failure=False):
    """Reload a system service, optionally falling back to restart if
    reload fails"""
    service_result = service('reload', service_name)
    if not service_result and restart_on_failure:
        service_result = service('restart', service_name)
    return service_result


def service(action, service_name):
    """Control a system service"""
    cmd = ['service', service_name, action]
    return subprocess.call(cmd) == 0


def service_running(service):
    """Determine whether a system service is running"""
    try:
        output = subprocess.check_output(
            ['service', service, 'status'],
            stderr=subprocess.STDOUT).decode('UTF-8')
    except subprocess.CalledProcessError:
        return False
    else:
        if ("start/running" in output or "is running" in output):
            return True
        else:
            return False


def service_available(service_name):
    """Determine whether a system service is available"""
    try:
        subprocess.check_output(
            ['service', service_name, 'status'],
            stderr=subprocess.STDOUT).decode('UTF-8')
    except subprocess.CalledProcessError as e:
        return b'unrecognized service' not in e.output
    else:
        return True


def adduser(username, password=None, shell='/bin/bash', system_user=False):
    """Add a user to the system"""
    try:
        user_info = pwd.getpwnam(username)
        log('user {0} already exists!'.format(username))
    except KeyError:
        log('creating user {0}'.format(username))
        cmd = ['useradd']
        if system_user or password is None:
            cmd.append('--system')
        else:
            cmd.extend([
                '--create-home',
                '--shell', shell,
                '--password', password,
            ])
        cmd.append(username)
        subprocess.check_call(cmd)
        user_info = pwd.getpwnam(username)
    return user_info


def add_group(group_name, system_group=False):
    """Add a group to the system"""
    try:
        group_info = grp.getgrnam(group_name)
        log('group {0} already exists!'.format(group_name))
    except KeyError:
        log('creating group {0}'.format(group_name))
        cmd = ['addgroup']
        if system_group:
            cmd.append('--system')
        else:
            cmd.extend([
                '--group',
            ])
        cmd.append(group_name)
        subprocess.check_call(cmd)
        group_info = grp.getgrnam(group_name)
    return group_info


def add_user_to_group(username, group):
    """Add a user to a group"""
    cmd = [
        'gpasswd', '-a',
        username,
        group
    ]
    log("Adding user {} to group {}".format(username, group))
    subprocess.check_call(cmd)


def rsync(from_path, to_path, flags='-r', options=None):
    """Replicate the contents of a path"""
    options = options or ['--delete', '--executability']
    cmd = ['/usr/bin/rsync', flags]
    cmd.extend(options)
    cmd.append(from_path)
    cmd.append(to_path)
    log(" ".join(cmd))
    return subprocess.check_output(cmd).decode('UTF-8').strip()


def symlink(source, destination):
    """Create a symbolic link"""
    log("Symlinking {} as {}".format(source, destination))
    cmd = [
        'ln',
        '-sf',
        source,
        destination,
    ]
    subprocess.check_call(cmd)


def mkdir(path, owner='root', group='root', perms=0o555, force=False):
    """Create a directory"""
    log("Making dir {} {}:{} {:o}".format(path, owner, group,
                                          perms))
    uid = pwd.getpwnam(owner).pw_uid
    gid = grp.getgrnam(group).gr_gid
    realpath = os.path.abspath(path)
    path_exists = os.path.exists(realpath)
    if path_exists and force:
        if not os.path.isdir(realpath):
            log("Removing non-directory file {} prior to mkdir()".format(path))
            os.unlink(realpath)
            os.makedirs(realpath, perms)
    elif not path_exists:
        os.makedirs(realpath, perms)
    os.chown(realpath, uid, gid)
    os.chmod(realpath, perms)


def write_file(path, content, owner='root', group='root', perms=0o444):
    """Create or overwrite a file with the contents of a byte string."""
    log("Writing file {} {}:{} {:o}".format(path, owner, group, perms))
    uid = pwd.getpwnam(owner).pw_uid
    gid = grp.getgrnam(group).gr_gid
    with open(path, 'wb') as target:
        os.fchown(target.fileno(), uid, gid)
        os.fchmod(target.fileno(), perms)
        target.write(content)


def fstab_remove(mp):
    """Remove the given mountpoint entry from /etc/fstab
    """
    return Fstab.remove_by_mountpoint(mp)


def fstab_add(dev, mp, fs, options=None):
    """Adds the given device entry to the /etc/fstab file
    """
    return Fstab.add(dev, mp, fs, options=options)


def mount(device, mountpoint, options=None, persist=False, filesystem="ext3"):
    """Mount a filesystem at a particular mountpoint"""
    cmd_args = ['mount']
    if options is not None:
        cmd_args.extend(['-o', options])
    cmd_args.extend([device, mountpoint])
    try:
        subprocess.check_output(cmd_args)
    except subprocess.CalledProcessError as e:
        log('Error mounting {} at {}\n{}'.format(device, mountpoint, e.output))
        return False

    if persist:
        return fstab_add(device, mountpoint, filesystem, options=options)
    return True


def umount(mountpoint, persist=False):
    """Unmount a filesystem"""
    cmd_args = ['umount', mountpoint]
    try:
        subprocess.check_output(cmd_args)
    except subprocess.CalledProcessError as e:
        log('Error unmounting {}\n{}'.format(mountpoint, e.output))
        return False

    if persist:
        return fstab_remove(mountpoint)
    return True


def mounts():
    """Get a list of all mounted volumes as [[mountpoint,device],[...]]"""
    with open('/proc/mounts') as f:
        # [['/mount/point','/dev/path'],[...]]
        system_mounts = [m[1::-1] for m in [l.strip().split()
                                            for l in f.readlines()]]
    return system_mounts


def file_hash(path, hash_type='md5'):
    """
    Generate a hash checksum of the contents of 'path' or None if not found.

    :param str hash_type: Any hash alrgorithm supported by :mod:`hashlib`,
                          such as md5, sha1, sha256, sha512, etc.
    """
    if os.path.exists(path):
        h = getattr(hashlib, hash_type)()
        with open(path, 'rb') as source:
            h.update(source.read())
        return h.hexdigest()
    else:
        return None


def check_hash(path, checksum, hash_type='md5'):
    """
    Validate a file using a cryptographic checksum.

    :param str checksum: Value of the checksum used to validate the file.
    :param str hash_type: Hash algorithm used to generate `checksum`.
        Can be any hash alrgorithm supported by :mod:`hashlib`,
        such as md5, sha1, sha256, sha512, etc.
    :raises ChecksumError: If the file fails the checksum

    """
    actual_checksum = file_hash(path, hash_type)
    if checksum != actual_checksum:
        raise ChecksumError("'%s' != '%s'" % (checksum, actual_checksum))


class ChecksumError(ValueError):
    pass


def restart_on_change(restart_map, stopstart=False):
    """Restart services based on configuration files changing

    This function is used a decorator, for example::

        @restart_on_change({
            '/etc/ceph/ceph.conf': [ 'cinder-api', 'cinder-volume' ]
            })
        def ceph_client_changed():
            pass  # your code here

    In this example, the cinder-api and cinder-volume services
    would be restarted if /etc/ceph/ceph.conf is changed by the
    ceph_client_changed function.
    """
    def wrap(f):
        def wrapped_f(*args, **kwargs):
            checksums = {}
            for path in restart_map:
                checksums[path] = file_hash(path)
            f(*args, **kwargs)
            restarts = []
            for path in restart_map:
                if checksums[path] != file_hash(path):
                    restarts += restart_map[path]
            services_list = list(OrderedDict.fromkeys(restarts))
            if not stopstart:
                for service_name in services_list:
                    service('restart', service_name)
            else:
                for action in ['stop', 'start']:
                    for service_name in services_list:
                        service(action, service_name)
        return wrapped_f
    return wrap


def lsb_release():
    """Return /etc/lsb-release in a dict"""
    d = {}
    with open('/etc/lsb-release', 'r') as lsb:
        for l in lsb:
            k, v = l.split('=')
            d[k.strip()] = v.strip()
    return d


def pwgen(length=None):
    """Generate a random pasword."""
    if length is None:
        # A random length is ok to use a weak PRNG
        length = random.choice(range(35, 45))
    alphanumeric_chars = [
        l for l in (string.ascii_letters + string.digits)
        if l not in 'l0QD1vAEIOUaeiou']
    # Use a crypto-friendly PRNG (e.g. /dev/urandom) for making the
    # actual password
    random_generator = random.SystemRandom()
    random_chars = [
        random_generator.choice(alphanumeric_chars) for _ in range(length)]
    return(''.join(random_chars))


def list_nics(nic_type):
    '''Return a list of nics of given type(s)'''
    if isinstance(nic_type, six.string_types):
        int_types = [nic_type]
    else:
        int_types = nic_type
    interfaces = []
    for int_type in int_types:
        cmd = ['ip', 'addr', 'show', 'label', int_type + '*']
        ip_output = subprocess.check_output(cmd).decode('UTF-8').split('\n')
        ip_output = (line for line in ip_output if line)
        for line in ip_output:
            if line.split()[1].startswith(int_type):
                matched = re.search('.*: (' + int_type + r'[0-9]+\.[0-9]+)@.*', line)
                if matched:
                    interface = matched.groups()[0]
                else:
                    interface = line.split()[1].replace(":", "")
                interfaces.append(interface)

    return interfaces


def set_nic_mtu(nic, mtu):
    '''Set MTU on a network interface'''
    cmd = ['ip', 'link', 'set', nic, 'mtu', mtu]
    subprocess.check_call(cmd)


def get_nic_mtu(nic):
    cmd = ['ip', 'addr', 'show', nic]
    ip_output = subprocess.check_output(cmd).decode('UTF-8').split('\n')
    mtu = ""
    for line in ip_output:
        words = line.split()
        if 'mtu' in words:
            mtu = words[words.index("mtu") + 1]
    return mtu


def get_nic_hwaddr(nic):
    cmd = ['ip', '-o', '-0', 'addr', 'show', nic]
    ip_output = subprocess.check_output(cmd).decode('UTF-8')
    hwaddr = ""
    words = ip_output.split()
    if 'link/ether' in words:
        hwaddr = words[words.index('link/ether') + 1]
    return hwaddr


def cmp_pkgrevno(package, revno, pkgcache=None):
    '''Compare supplied revno with the revno of the installed package

    *  1 => Installed revno is greater than supplied arg
    *  0 => Installed revno is the same as supplied arg
    * -1 => Installed revno is less than supplied arg

    This function imports apt_cache function from charmhelpers.fetch if
    the pkgcache argument is None. Be sure to add charmhelpers.fetch if
    you call this function, or pass an apt_pkg.Cache() instance.
    '''
    import apt_pkg
    if not pkgcache:
        from charmhelpers.fetch import apt_cache
        pkgcache = apt_cache()
    pkg = pkgcache[package]
    return apt_pkg.version_compare(pkg.current_ver.ver_str, revno)


@contextmanager
def chdir(d):
    cur = os.getcwd()
    try:
        yield os.chdir(d)
    finally:
        os.chdir(cur)


def chownr(path, owner, group, follow_links=True):
    uid = pwd.getpwnam(owner).pw_uid
    gid = grp.getgrnam(group).gr_gid
    if follow_links:
        chown = os.chown
    else:
        chown = os.lchown

    for root, dirs, files in os.walk(path):
        for name in dirs + files:
            full = os.path.join(root, name)
            broken_symlink = os.path.lexists(full) and not os.path.exists(full)
            if not broken_symlink:
                chown(full, uid, gid)


def lchownr(path, owner, group):
    chownr(path, owner, group, follow_links=False)
