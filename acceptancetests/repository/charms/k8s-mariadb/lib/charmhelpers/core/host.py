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

"""Tools for working with the host system"""
# Copyright 2012 Canonical Ltd.
#
# Authors:
#  Nick Moffitt <nick.moffitt@canonical.com>
#  Matthew Wedgwood <matthew.wedgwood@canonical.com>

import os
import re
import pwd
import glob
import grp
import random
import string
import subprocess
import hashlib
import functools
import itertools
import six

from contextlib import contextmanager
from collections import OrderedDict
from .hookenv import log, INFO, DEBUG, local_unit, charm_name
from .fstab import Fstab
from charmhelpers.osplatform import get_platform

__platform__ = get_platform()
if __platform__ == "ubuntu":
    from charmhelpers.core.host_factory.ubuntu import (  # NOQA:F401
        service_available,
        add_new_group,
        lsb_release,
        cmp_pkgrevno,
        CompareHostReleases,
    )  # flake8: noqa -- ignore F401 for this import
elif __platform__ == "centos":
    from charmhelpers.core.host_factory.centos import (  # NOQA:F401
        service_available,
        add_new_group,
        lsb_release,
        cmp_pkgrevno,
        CompareHostReleases,
    )  # flake8: noqa -- ignore F401 for this import

UPDATEDB_PATH = '/etc/updatedb.conf'


def service_start(service_name, **kwargs):
    """Start a system service.

    The specified service name is managed via the system level init system.
    Some init systems (e.g. upstart) require that additional arguments be
    provided in order to directly control service instances whereas other init
    systems allow for addressing instances of a service directly by name (e.g.
    systemd).

    The kwargs allow for the additional parameters to be passed to underlying
    init systems for those systems which require/allow for them. For example,
    the ceph-osd upstart script requires the id parameter to be passed along
    in order to identify which running daemon should be reloaded. The follow-
    ing example stops the ceph-osd service for instance id=4:

    service_stop('ceph-osd', id=4)

    :param service_name: the name of the service to stop
    :param **kwargs: additional parameters to pass to the init system when
                     managing services. These will be passed as key=value
                     parameters to the init system's commandline. kwargs
                     are ignored for systemd enabled systems.
    """
    return service('start', service_name, **kwargs)


def service_stop(service_name, **kwargs):
    """Stop a system service.

    The specified service name is managed via the system level init system.
    Some init systems (e.g. upstart) require that additional arguments be
    provided in order to directly control service instances whereas other init
    systems allow for addressing instances of a service directly by name (e.g.
    systemd).

    The kwargs allow for the additional parameters to be passed to underlying
    init systems for those systems which require/allow for them. For example,
    the ceph-osd upstart script requires the id parameter to be passed along
    in order to identify which running daemon should be reloaded. The follow-
    ing example stops the ceph-osd service for instance id=4:

    service_stop('ceph-osd', id=4)

    :param service_name: the name of the service to stop
    :param **kwargs: additional parameters to pass to the init system when
                     managing services. These will be passed as key=value
                     parameters to the init system's commandline. kwargs
                     are ignored for systemd enabled systems.
    """
    return service('stop', service_name, **kwargs)


def service_restart(service_name, **kwargs):
    """Restart a system service.

    The specified service name is managed via the system level init system.
    Some init systems (e.g. upstart) require that additional arguments be
    provided in order to directly control service instances whereas other init
    systems allow for addressing instances of a service directly by name (e.g.
    systemd).

    The kwargs allow for the additional parameters to be passed to underlying
    init systems for those systems which require/allow for them. For example,
    the ceph-osd upstart script requires the id parameter to be passed along
    in order to identify which running daemon should be restarted. The follow-
    ing example restarts the ceph-osd service for instance id=4:

    service_restart('ceph-osd', id=4)

    :param service_name: the name of the service to restart
    :param **kwargs: additional parameters to pass to the init system when
                     managing services. These will be passed as key=value
                     parameters to the  init system's commandline. kwargs
                     are ignored for init systems not allowing additional
                     parameters via the commandline (systemd).
    """
    return service('restart', service_name)


def service_reload(service_name, restart_on_failure=False, **kwargs):
    """Reload a system service, optionally falling back to restart if
    reload fails.

    The specified service name is managed via the system level init system.
    Some init systems (e.g. upstart) require that additional arguments be
    provided in order to directly control service instances whereas other init
    systems allow for addressing instances of a service directly by name (e.g.
    systemd).

    The kwargs allow for the additional parameters to be passed to underlying
    init systems for those systems which require/allow for them. For example,
    the ceph-osd upstart script requires the id parameter to be passed along
    in order to identify which running daemon should be reloaded. The follow-
    ing example restarts the ceph-osd service for instance id=4:

    service_reload('ceph-osd', id=4)

    :param service_name: the name of the service to reload
    :param restart_on_failure: boolean indicating whether to fallback to a
                               restart if the reload fails.
    :param **kwargs: additional parameters to pass to the init system when
                     managing services. These will be passed as key=value
                     parameters to the  init system's commandline. kwargs
                     are ignored for init systems not allowing additional
                     parameters via the commandline (systemd).
    """
    service_result = service('reload', service_name, **kwargs)
    if not service_result and restart_on_failure:
        service_result = service('restart', service_name, **kwargs)
    return service_result


def service_pause(service_name, init_dir="/etc/init", initd_dir="/etc/init.d",
                  **kwargs):
    """Pause a system service.

    Stop it, and prevent it from starting again at boot.

    :param service_name: the name of the service to pause
    :param init_dir: path to the upstart init directory
    :param initd_dir: path to the sysv init directory
    :param **kwargs: additional parameters to pass to the init system when
                     managing services. These will be passed as key=value
                     parameters to the init system's commandline. kwargs
                     are ignored for init systems which do not support
                     key=value arguments via the commandline.
    """
    stopped = True
    if service_running(service_name, **kwargs):
        stopped = service_stop(service_name, **kwargs)
    upstart_file = os.path.join(init_dir, "{}.conf".format(service_name))
    sysv_file = os.path.join(initd_dir, service_name)
    if init_is_systemd():
        service('disable', service_name)
        service('mask', service_name)
    elif os.path.exists(upstart_file):
        override_path = os.path.join(
            init_dir, '{}.override'.format(service_name))
        with open(override_path, 'w') as fh:
            fh.write("manual\n")
    elif os.path.exists(sysv_file):
        subprocess.check_call(["update-rc.d", service_name, "disable"])
    else:
        raise ValueError(
            "Unable to detect {0} as SystemD, Upstart {1} or"
            " SysV {2}".format(
                service_name, upstart_file, sysv_file))
    return stopped


def service_resume(service_name, init_dir="/etc/init",
                   initd_dir="/etc/init.d", **kwargs):
    """Resume a system service.

    Reenable starting again at boot. Start the service.

    :param service_name: the name of the service to resume
    :param init_dir: the path to the init dir
    :param initd dir: the path to the initd dir
    :param **kwargs: additional parameters to pass to the init system when
                     managing services. These will be passed as key=value
                     parameters to the init system's commandline. kwargs
                     are ignored for systemd enabled systems.
    """
    upstart_file = os.path.join(init_dir, "{}.conf".format(service_name))
    sysv_file = os.path.join(initd_dir, service_name)
    if init_is_systemd():
        service('unmask', service_name)
        service('enable', service_name)
    elif os.path.exists(upstart_file):
        override_path = os.path.join(
            init_dir, '{}.override'.format(service_name))
        if os.path.exists(override_path):
            os.unlink(override_path)
    elif os.path.exists(sysv_file):
        subprocess.check_call(["update-rc.d", service_name, "enable"])
    else:
        raise ValueError(
            "Unable to detect {0} as SystemD, Upstart {1} or"
            " SysV {2}".format(
                service_name, upstart_file, sysv_file))
    started = service_running(service_name, **kwargs)

    if not started:
        started = service_start(service_name, **kwargs)
    return started


def service(action, service_name, **kwargs):
    """Control a system service.

    :param action: the action to take on the service
    :param service_name: the name of the service to perform th action on
    :param **kwargs: additional params to be passed to the service command in
                    the form of key=value.
    """
    if init_is_systemd():
        cmd = ['systemctl', action, service_name]
    else:
        cmd = ['service', service_name, action]
        for key, value in six.iteritems(kwargs):
            parameter = '%s=%s' % (key, value)
            cmd.append(parameter)
    return subprocess.call(cmd) == 0


_UPSTART_CONF = "/etc/init/{}.conf"
_INIT_D_CONF = "/etc/init.d/{}"


def service_running(service_name, **kwargs):
    """Determine whether a system service is running.

    :param service_name: the name of the service
    :param **kwargs: additional args to pass to the service command. This is
                     used to pass additional key=value arguments to the
                     service command line for managing specific instance
                     units (e.g. service ceph-osd status id=2). The kwargs
                     are ignored in systemd services.
    """
    if init_is_systemd():
        return service('is-active', service_name)
    else:
        if os.path.exists(_UPSTART_CONF.format(service_name)):
            try:
                cmd = ['status', service_name]
                for key, value in six.iteritems(kwargs):
                    parameter = '%s=%s' % (key, value)
                    cmd.append(parameter)
                output = subprocess.check_output(
                    cmd, stderr=subprocess.STDOUT).decode('UTF-8')
            except subprocess.CalledProcessError:
                return False
            else:
                # This works for upstart scripts where the 'service' command
                # returns a consistent string to represent running
                # 'start/running'
                if ("start/running" in output or
                        "is running" in output or
                        "up and running" in output):
                    return True
        elif os.path.exists(_INIT_D_CONF.format(service_name)):
            # Check System V scripts init script return codes
            return service('status', service_name)
        return False


SYSTEMD_SYSTEM = '/run/systemd/system'


def init_is_systemd():
    """Return True if the host system uses systemd, False otherwise."""
    if lsb_release()['DISTRIB_CODENAME'] == 'trusty':
        return False
    return os.path.isdir(SYSTEMD_SYSTEM)


def adduser(username, password=None, shell='/bin/bash',
            system_user=False, primary_group=None,
            secondary_groups=None, uid=None, home_dir=None):
    """Add a user to the system.

    Will log but otherwise succeed if the user already exists.

    :param str username: Username to create
    :param str password: Password for user; if ``None``, create a system user
    :param str shell: The default shell for the user
    :param bool system_user: Whether to create a login or system user
    :param str primary_group: Primary group for user; defaults to username
    :param list secondary_groups: Optional list of additional groups
    :param int uid: UID for user being created
    :param str home_dir: Home directory for user

    :returns: The password database entry struct, as returned by `pwd.getpwnam`
    """
    try:
        user_info = pwd.getpwnam(username)
        log('user {0} already exists!'.format(username))
        if uid:
            user_info = pwd.getpwuid(int(uid))
            log('user with uid {0} already exists!'.format(uid))
    except KeyError:
        log('creating user {0}'.format(username))
        cmd = ['useradd']
        if uid:
            cmd.extend(['--uid', str(uid)])
        if home_dir:
            cmd.extend(['--home', str(home_dir)])
        if system_user or password is None:
            cmd.append('--system')
        else:
            cmd.extend([
                '--create-home',
                '--shell', shell,
                '--password', password,
            ])
        if not primary_group:
            try:
                grp.getgrnam(username)
                primary_group = username  # avoid "group exists" error
            except KeyError:
                pass
        if primary_group:
            cmd.extend(['-g', primary_group])
        if secondary_groups:
            cmd.extend(['-G', ','.join(secondary_groups)])
        cmd.append(username)
        subprocess.check_call(cmd)
        user_info = pwd.getpwnam(username)
    return user_info


def user_exists(username):
    """Check if a user exists"""
    try:
        pwd.getpwnam(username)
        user_exists = True
    except KeyError:
        user_exists = False
    return user_exists


def uid_exists(uid):
    """Check if a uid exists"""
    try:
        pwd.getpwuid(uid)
        uid_exists = True
    except KeyError:
        uid_exists = False
    return uid_exists


def group_exists(groupname):
    """Check if a group exists"""
    try:
        grp.getgrnam(groupname)
        group_exists = True
    except KeyError:
        group_exists = False
    return group_exists


def gid_exists(gid):
    """Check if a gid exists"""
    try:
        grp.getgrgid(gid)
        gid_exists = True
    except KeyError:
        gid_exists = False
    return gid_exists


def add_group(group_name, system_group=False, gid=None):
    """Add a group to the system

    Will log but otherwise succeed if the group already exists.

    :param str group_name: group to create
    :param bool system_group: Create system group
    :param int gid: GID for user being created

    :returns: The password database entry struct, as returned by `grp.getgrnam`
    """
    try:
        group_info = grp.getgrnam(group_name)
        log('group {0} already exists!'.format(group_name))
        if gid:
            group_info = grp.getgrgid(gid)
            log('group with gid {0} already exists!'.format(gid))
    except KeyError:
        log('creating group {0}'.format(group_name))
        add_new_group(group_name, system_group, gid)
        group_info = grp.getgrnam(group_name)
    return group_info


def add_user_to_group(username, group):
    """Add a user to a group"""
    cmd = ['gpasswd', '-a', username, group]
    log("Adding user {} to group {}".format(username, group))
    subprocess.check_call(cmd)


def chage(username, lastday=None, expiredate=None, inactive=None,
          mindays=None, maxdays=None, root=None, warndays=None):
    """Change user password expiry information

    :param str username: User to update
    :param str lastday: Set when password was changed in YYYY-MM-DD format
    :param str expiredate: Set when user's account will no longer be
                           accessible in YYYY-MM-DD format.
                           -1 will remove an account expiration date.
    :param str inactive: Set the number of days of inactivity after a password
                         has expired before the account is locked.
                         -1 will remove an account's inactivity.
    :param str mindays: Set the minimum number of days between password
                        changes to MIN_DAYS.
                        0 indicates the password can be changed anytime.
    :param str maxdays: Set the maximum number of days during which a
                        password is valid.
                        -1 as MAX_DAYS will remove checking maxdays
    :param str root: Apply changes in the CHROOT_DIR directory
    :param str warndays: Set the number of days of warning before a password
                         change is required
    :raises subprocess.CalledProcessError: if call to chage fails
    """
    cmd = ['chage']
    if root:
        cmd.extend(['--root', root])
    if lastday:
        cmd.extend(['--lastday', lastday])
    if expiredate:
        cmd.extend(['--expiredate', expiredate])
    if inactive:
        cmd.extend(['--inactive', inactive])
    if mindays:
        cmd.extend(['--mindays', mindays])
    if maxdays:
        cmd.extend(['--maxdays', maxdays])
    if warndays:
        cmd.extend(['--warndays', warndays])
    cmd.append(username)
    subprocess.check_call(cmd)


remove_password_expiry = functools.partial(chage, expiredate='-1', inactive='-1', mindays='0', maxdays='-1')


def rsync(from_path, to_path, flags='-r', options=None, timeout=None):
    """Replicate the contents of a path"""
    options = options or ['--delete', '--executability']
    cmd = ['/usr/bin/rsync', flags]
    if timeout:
        cmd = ['timeout', str(timeout)] + cmd
    cmd.extend(options)
    cmd.append(from_path)
    cmd.append(to_path)
    log(" ".join(cmd))
    return subprocess.check_output(cmd, stderr=subprocess.STDOUT).decode('UTF-8').strip()


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
    uid = pwd.getpwnam(owner).pw_uid
    gid = grp.getgrnam(group).gr_gid
    # lets see if we can grab the file and compare the context, to avoid doing
    # a write.
    existing_content = None
    existing_uid, existing_gid, existing_perms = None, None, None
    try:
        with open(path, 'rb') as target:
            existing_content = target.read()
        stat = os.stat(path)
        existing_uid, existing_gid, existing_perms = (
            stat.st_uid, stat.st_gid, stat.st_mode
        )
    except Exception:
        pass
    if content != existing_content:
        log("Writing file {} {}:{} {:o}".format(path, owner, group, perms),
            level=DEBUG)
        with open(path, 'wb') as target:
            os.fchown(target.fileno(), uid, gid)
            os.fchmod(target.fileno(), perms)
            if six.PY3 and isinstance(content, six.string_types):
                content = content.encode('UTF-8')
            target.write(content)
        return
    # the contents were the same, but we might still need to change the
    # ownership or permissions.
    if existing_uid != uid:
        log("Changing uid on already existing content: {} -> {}"
            .format(existing_uid, uid), level=DEBUG)
        os.chown(path, uid, -1)
    if existing_gid != gid:
        log("Changing gid on already existing content: {} -> {}"
            .format(existing_gid, gid), level=DEBUG)
        os.chown(path, -1, gid)
    if existing_perms != perms:
        log("Changing permissions on existing content: {} -> {}"
            .format(existing_perms, perms), level=DEBUG)
        os.chmod(path, perms)


def fstab_remove(mp):
    """Remove the given mountpoint entry from /etc/fstab"""
    return Fstab.remove_by_mountpoint(mp)


def fstab_add(dev, mp, fs, options=None):
    """Adds the given device entry to the /etc/fstab file"""
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


def fstab_mount(mountpoint):
    """Mount filesystem using fstab"""
    cmd_args = ['mount', mountpoint]
    try:
        subprocess.check_output(cmd_args)
    except subprocess.CalledProcessError as e:
        log('Error unmounting {}\n{}'.format(mountpoint, e.output))
        return False
    return True


def file_hash(path, hash_type='md5'):
    """Generate a hash checksum of the contents of 'path' or None if not found.

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


def path_hash(path):
    """Generate a hash checksum of all files matching 'path'. Standard
    wildcards like '*' and '?' are supported, see documentation for the 'glob'
    module for more information.

    :return: dict: A { filename: hash } dictionary for all matched files.
                   Empty if none found.
    """
    return {
        filename: file_hash(filename)
        for filename in glob.iglob(path)
    }


def check_hash(path, checksum, hash_type='md5'):
    """Validate a file using a cryptographic checksum.

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
    """A class derived from Value error to indicate the checksum failed."""
    pass


def restart_on_change(restart_map, stopstart=False, restart_functions=None):
    """Restart services based on configuration files changing

    This function is used a decorator, for example::

        @restart_on_change({
            '/etc/ceph/ceph.conf': [ 'cinder-api', 'cinder-volume' ]
            '/etc/apache/sites-enabled/*': [ 'apache2' ]
            })
        def config_changed():
            pass  # your code here

    In this example, the cinder-api and cinder-volume services
    would be restarted if /etc/ceph/ceph.conf is changed by the
    ceph_client_changed function. The apache2 service would be
    restarted if any file matching the pattern got changed, created
    or removed. Standard wildcards are supported, see documentation
    for the 'glob' module for more information.

    @param restart_map: {path_file_name: [service_name, ...]
    @param stopstart: DEFAULT false; whether to stop, start OR restart
    @param restart_functions: nonstandard functions to use to restart services
                              {svc: func, ...}
    @returns result from decorated function
    """
    def wrap(f):
        @functools.wraps(f)
        def wrapped_f(*args, **kwargs):
            return restart_on_change_helper(
                (lambda: f(*args, **kwargs)), restart_map, stopstart,
                restart_functions)
        return wrapped_f
    return wrap


def restart_on_change_helper(lambda_f, restart_map, stopstart=False,
                             restart_functions=None):
    """Helper function to perform the restart_on_change function.

    This is provided for decorators to restart services if files described
    in the restart_map have changed after an invocation of lambda_f().

    @param lambda_f: function to call.
    @param restart_map: {file: [service, ...]}
    @param stopstart: whether to stop, start or restart a service
    @param restart_functions: nonstandard functions to use to restart services
                              {svc: func, ...}
    @returns result of lambda_f()
    """
    if restart_functions is None:
        restart_functions = {}
    checksums = {path: path_hash(path) for path in restart_map}
    r = lambda_f()
    # create a list of lists of the services to restart
    restarts = [restart_map[path]
                for path in restart_map
                if path_hash(path) != checksums[path]]
    # create a flat list of ordered services without duplicates from lists
    services_list = list(OrderedDict.fromkeys(itertools.chain(*restarts)))
    if services_list:
        actions = ('stop', 'start') if stopstart else ('restart',)
        for service_name in services_list:
            if service_name in restart_functions:
                restart_functions[service_name](service_name)
            else:
                for action in actions:
                    service(action, service_name)
    return r


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


def is_phy_iface(interface):
    """Returns True if interface is not virtual, otherwise False."""
    if interface:
        sys_net = '/sys/class/net'
        if os.path.isdir(sys_net):
            for iface in glob.glob(os.path.join(sys_net, '*')):
                if '/virtual/' in os.path.realpath(iface):
                    continue

                if interface == os.path.basename(iface):
                    return True

    return False


def get_bond_master(interface):
    """Returns bond master if interface is bond slave otherwise None.

    NOTE: the provided interface is expected to be physical
    """
    if interface:
        iface_path = '/sys/class/net/%s' % (interface)
        if os.path.exists(iface_path):
            if '/virtual/' in os.path.realpath(iface_path):
                return None

            master = os.path.join(iface_path, 'master')
            if os.path.exists(master):
                master = os.path.realpath(master)
                # make sure it is a bond master
                if os.path.exists(os.path.join(master, 'bonding')):
                    return os.path.basename(master)

    return None


def list_nics(nic_type=None):
    """Return a list of nics of given type(s)"""
    if isinstance(nic_type, six.string_types):
        int_types = [nic_type]
    else:
        int_types = nic_type

    interfaces = []
    if nic_type:
        for int_type in int_types:
            cmd = ['ip', 'addr', 'show', 'label', int_type + '*']
            ip_output = subprocess.check_output(cmd).decode('UTF-8')
            ip_output = ip_output.split('\n')
            ip_output = (line for line in ip_output if line)
            for line in ip_output:
                if line.split()[1].startswith(int_type):
                    matched = re.search('.*: (' + int_type +
                                        r'[0-9]+\.[0-9]+)@.*', line)
                    if matched:
                        iface = matched.groups()[0]
                    else:
                        iface = line.split()[1].replace(":", "")

                    if iface not in interfaces:
                        interfaces.append(iface)
    else:
        cmd = ['ip', 'a']
        ip_output = subprocess.check_output(cmd).decode('UTF-8').split('\n')
        ip_output = (line.strip() for line in ip_output if line)

        key = re.compile(r'^[0-9]+:\s+(.+):')
        for line in ip_output:
            matched = re.search(key, line)
            if matched:
                iface = matched.group(1)
                iface = iface.partition("@")[0]
                if iface not in interfaces:
                    interfaces.append(iface)

    return interfaces


def set_nic_mtu(nic, mtu):
    """Set the Maximum Transmission Unit (MTU) on a network interface."""
    cmd = ['ip', 'link', 'set', nic, 'mtu', mtu]
    subprocess.check_call(cmd)


def get_nic_mtu(nic):
    """Return the Maximum Transmission Unit (MTU) for a network interface."""
    cmd = ['ip', 'addr', 'show', nic]
    ip_output = subprocess.check_output(cmd).decode('UTF-8').split('\n')
    mtu = ""
    for line in ip_output:
        words = line.split()
        if 'mtu' in words:
            mtu = words[words.index("mtu") + 1]
    return mtu


def get_nic_hwaddr(nic):
    """Return the Media Access Control (MAC) for a network interface."""
    cmd = ['ip', '-o', '-0', 'addr', 'show', nic]
    ip_output = subprocess.check_output(cmd).decode('UTF-8')
    hwaddr = ""
    words = ip_output.split()
    if 'link/ether' in words:
        hwaddr = words[words.index('link/ether') + 1]
    return hwaddr


@contextmanager
def chdir(directory):
    """Change the current working directory to a different directory for a code
    block and return the previous directory after the block exits. Useful to
    run commands from a specificed directory.

    :param str directory: The directory path to change to for this context.
    """
    cur = os.getcwd()
    try:
        yield os.chdir(directory)
    finally:
        os.chdir(cur)


def chownr(path, owner, group, follow_links=True, chowntopdir=False):
    """Recursively change user and group ownership of files and directories
    in given path. Doesn't chown path itself by default, only its children.

    :param str path: The string path to start changing ownership.
    :param str owner: The owner string to use when looking up the uid.
    :param str group: The group string to use when looking up the gid.
    :param bool follow_links: Also follow and chown links if True
    :param bool chowntopdir: Also chown path itself if True
    """
    uid = pwd.getpwnam(owner).pw_uid
    gid = grp.getgrnam(group).gr_gid
    if follow_links:
        chown = os.chown
    else:
        chown = os.lchown

    if chowntopdir:
        broken_symlink = os.path.lexists(path) and not os.path.exists(path)
        if not broken_symlink:
            chown(path, uid, gid)
    for root, dirs, files in os.walk(path, followlinks=follow_links):
        for name in dirs + files:
            full = os.path.join(root, name)
            broken_symlink = os.path.lexists(full) and not os.path.exists(full)
            if not broken_symlink:
                chown(full, uid, gid)


def lchownr(path, owner, group):
    """Recursively change user and group ownership of files and directories
    in a given path, not following symbolic links. See the documentation for
    'os.lchown' for more information.

    :param str path: The string path to start changing ownership.
    :param str owner: The owner string to use when looking up the uid.
    :param str group: The group string to use when looking up the gid.
    """
    chownr(path, owner, group, follow_links=False)


def owner(path):
    """Returns a tuple containing the username & groupname owning the path.

    :param str path: the string path to retrieve the ownership
    :return tuple(str, str): A (username, groupname) tuple containing the
                             name of the user and group owning the path.
    :raises OSError: if the specified path does not exist
    """
    stat = os.stat(path)
    username = pwd.getpwuid(stat.st_uid)[0]
    groupname = grp.getgrgid(stat.st_gid)[0]
    return username, groupname


def get_total_ram():
    """The total amount of system RAM in bytes.

    This is what is reported by the OS, and may be overcommitted when
    there are multiple containers hosted on the same machine.
    """
    with open('/proc/meminfo', 'r') as f:
        for line in f.readlines():
            if line:
                key, value, unit = line.split()
                if key == 'MemTotal:':
                    assert unit == 'kB', 'Unknown unit'
                    return int(value) * 1024  # Classic, not KiB.
        raise NotImplementedError()


UPSTART_CONTAINER_TYPE = '/run/container_type'


def is_container():
    """Determine whether unit is running in a container

    @return: boolean indicating if unit is in a container
    """
    if init_is_systemd():
        # Detect using systemd-detect-virt
        return subprocess.call(['systemd-detect-virt',
                                '--container']) == 0
    else:
        # Detect using upstart container file marker
        return os.path.exists(UPSTART_CONTAINER_TYPE)


def add_to_updatedb_prunepath(path, updatedb_path=UPDATEDB_PATH):
    """Adds the specified path to the mlocate's udpatedb.conf PRUNEPATH list.

    This method has no effect if the path specified by updatedb_path does not
    exist or is not a file.

    @param path: string the path to add to the updatedb.conf PRUNEPATHS value
    @param updatedb_path: the path the updatedb.conf file
    """
    if not os.path.exists(updatedb_path) or os.path.isdir(updatedb_path):
        # If the updatedb.conf file doesn't exist then don't attempt to update
        # the file as the package providing mlocate may not be installed on
        # the local system
        return

    with open(updatedb_path, 'r+') as f_id:
        updatedb_text = f_id.read()
        output = updatedb(updatedb_text, path)
        f_id.seek(0)
        f_id.write(output)
        f_id.truncate()


def updatedb(updatedb_text, new_path):
    lines = [line for line in updatedb_text.split("\n")]
    for i, line in enumerate(lines):
        if line.startswith("PRUNEPATHS="):
            paths_line = line.split("=")[1].replace('"', '')
            paths = paths_line.split(" ")
            if new_path not in paths:
                paths.append(new_path)
                lines[i] = 'PRUNEPATHS="{}"'.format(' '.join(paths))
    output = "\n".join(lines)
    return output


def modulo_distribution(modulo=3, wait=30, non_zero_wait=False):
    """ Modulo distribution

    This helper uses the unit number, a modulo value and a constant wait time
    to produce a calculated wait time distribution. This is useful in large
    scale deployments to distribute load during an expensive operation such as
    service restarts.

    If you have 1000 nodes that need to restart 100 at a time 1 minute at a
    time:

      time.wait(modulo_distribution(modulo=100, wait=60))
      restart()

    If you need restarts to happen serially set modulo to the exact number of
    nodes and set a high constant wait time:

      time.wait(modulo_distribution(modulo=10, wait=120))
      restart()

    @param modulo: int The modulo number creates the group distribution
    @param wait: int The constant time wait value
    @param non_zero_wait: boolean Override unit % modulo == 0,
                          return modulo * wait. Used to avoid collisions with
                          leader nodes which are often given priority.
    @return: int Calculated time to wait for unit operation
    """
    unit_number = int(local_unit().split('/')[1])
    calculated_wait_time = (unit_number % modulo) * wait
    if non_zero_wait and calculated_wait_time == 0:
        return modulo * wait
    else:
        return calculated_wait_time


def install_ca_cert(ca_cert, name=None):
    """
    Install the given cert as a trusted CA.

    The ``name`` is the stem of the filename where the cert is written, and if
    not provided, it will default to ``juju-{charm_name}``.

    If the cert is empty or None, or is unchanged, nothing is done.
    """
    if not ca_cert:
        return
    if not isinstance(ca_cert, bytes):
        ca_cert = ca_cert.encode('utf8')
    if not name:
        name = 'juju-{}'.format(charm_name())
    cert_file = '/usr/local/share/ca-certificates/{}.crt'.format(name)
    new_hash = hashlib.md5(ca_cert).hexdigest()
    if file_hash(cert_file) == new_hash:
        return
    log("Installing new CA cert at: {}".format(cert_file), level=INFO)
    write_file(cert_file, ca_cert)
    subprocess.check_call(['update-ca-certificates', '--fresh'])
