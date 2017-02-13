# Copyright 2016 Canonical Limited.
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

from charmhelpers.core.hookenv import (
    log,
    INFO,
)
from charmhelpers.contrib.hardening.audits.file import NoSUIDSGIDAudit
from charmhelpers.contrib.hardening import utils


BLACKLIST = ['/usr/bin/rcp', '/usr/bin/rlogin', '/usr/bin/rsh',
             '/usr/libexec/openssh/ssh-keysign',
             '/usr/lib/openssh/ssh-keysign',
             '/sbin/netreport',
             '/usr/sbin/usernetctl',
             '/usr/sbin/userisdnctl',
             '/usr/sbin/pppd',
             '/usr/bin/lockfile',
             '/usr/bin/mail-lock',
             '/usr/bin/mail-unlock',
             '/usr/bin/mail-touchlock',
             '/usr/bin/dotlockfile',
             '/usr/bin/arping',
             '/usr/sbin/uuidd',
             '/usr/bin/mtr',
             '/usr/lib/evolution/camel-lock-helper-1.2',
             '/usr/lib/pt_chown',
             '/usr/lib/eject/dmcrypt-get-device',
             '/usr/lib/mc/cons.saver']

WHITELIST = ['/bin/mount', '/bin/ping', '/bin/su', '/bin/umount',
             '/sbin/pam_timestamp_check', '/sbin/unix_chkpwd', '/usr/bin/at',
             '/usr/bin/gpasswd', '/usr/bin/locate', '/usr/bin/newgrp',
             '/usr/bin/passwd', '/usr/bin/ssh-agent',
             '/usr/libexec/utempter/utempter', '/usr/sbin/lockdev',
             '/usr/sbin/sendmail.sendmail', '/usr/bin/expiry',
             '/bin/ping6', '/usr/bin/traceroute6.iputils',
             '/sbin/mount.nfs', '/sbin/umount.nfs',
             '/sbin/mount.nfs4', '/sbin/umount.nfs4',
             '/usr/bin/crontab',
             '/usr/bin/wall', '/usr/bin/write',
             '/usr/bin/screen',
             '/usr/bin/mlocate',
             '/usr/bin/chage', '/usr/bin/chfn', '/usr/bin/chsh',
             '/bin/fusermount',
             '/usr/bin/pkexec',
             '/usr/bin/sudo', '/usr/bin/sudoedit',
             '/usr/sbin/postdrop', '/usr/sbin/postqueue',
             '/usr/sbin/suexec',
             '/usr/lib/squid/ncsa_auth', '/usr/lib/squid/pam_auth',
             '/usr/kerberos/bin/ksu',
             '/usr/sbin/ccreds_validate',
             '/usr/bin/Xorg',
             '/usr/bin/X',
             '/usr/lib/dbus-1.0/dbus-daemon-launch-helper',
             '/usr/lib/vte/gnome-pty-helper',
             '/usr/lib/libvte9/gnome-pty-helper',
             '/usr/lib/libvte-2.90-9/gnome-pty-helper']


def get_audits():
    """Get OS hardening suid/sgid audits.

    :returns:  dictionary of audits
    """
    checks = []
    settings = utils.get_settings('os')
    if not settings['security']['suid_sgid_enforce']:
        log("Skipping suid/sgid hardening", level=INFO)
        return checks

    # Build the blacklist and whitelist of files for suid/sgid checks.
    # There are a total of 4 lists:
    #   1. the system blacklist
    #   2. the system whitelist
    #   3. the user blacklist
    #   4. the user whitelist
    #
    # The blacklist is the set of paths which should NOT have the suid/sgid bit
    # set and the whitelist is the set of paths which MAY have the suid/sgid
    # bit setl. The user whitelist/blacklist effectively override the system
    # whitelist/blacklist.
    u_b = settings['security']['suid_sgid_blacklist']
    u_w = settings['security']['suid_sgid_whitelist']

    blacklist = set(BLACKLIST) - set(u_w + u_b)
    whitelist = set(WHITELIST) - set(u_b + u_w)

    checks.append(NoSUIDSGIDAudit(blacklist))

    dry_run = settings['security']['suid_sgid_dry_run_on_unknown']

    if settings['security']['suid_sgid_remove_from_unknown'] or dry_run:
        # If the policy is a dry_run (e.g. complain only) or remove unknown
        # suid/sgid bits then find all of the paths which have the suid/sgid
        # bit set and then remove the whitelisted paths.
        root_path = settings['environment']['root_path']
        unknown_paths = find_paths_with_suid_sgid(root_path) - set(whitelist)
        checks.append(NoSUIDSGIDAudit(unknown_paths, unless=dry_run))

    return checks


def find_paths_with_suid_sgid(root_path):
    """Finds all paths/files which have an suid/sgid bit enabled.

    Starting with the root_path, this will recursively find all paths which
    have an suid or sgid bit set.
    """
    cmd = ['find', root_path, '-perm', '-4000', '-o', '-perm', '-2000',
           '-type', 'f', '!', '-path', '/proc/*', '-print']

    p = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    out, _ = p.communicate()
    return set(out.split('\n'))
