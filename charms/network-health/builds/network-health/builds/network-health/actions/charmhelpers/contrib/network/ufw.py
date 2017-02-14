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

"""
This module contains helpers to add and remove ufw rules.

Examples:

- open SSH port for subnet 10.0.3.0/24:

  >>> from charmhelpers.contrib.network import ufw
  >>> ufw.enable()
  >>> ufw.grant_access(src='10.0.3.0/24', dst='any', port='22', proto='tcp')

- open service by name as defined in /etc/services:

  >>> from charmhelpers.contrib.network import ufw
  >>> ufw.enable()
  >>> ufw.service('ssh', 'open')

- close service by port number:

  >>> from charmhelpers.contrib.network import ufw
  >>> ufw.enable()
  >>> ufw.service('4949', 'close')  # munin
"""
import re
import os
import subprocess

from charmhelpers.core import hookenv
from charmhelpers.core.kernel import modprobe, is_module_loaded

__author__ = "Felipe Reyes <felipe.reyes@canonical.com>"


class UFWError(Exception):
    pass


class UFWIPv6Error(UFWError):
    pass


def is_enabled():
    """
    Check if `ufw` is enabled

    :returns: True if ufw is enabled
    """
    output = subprocess.check_output(['ufw', 'status'],
                                     universal_newlines=True,
                                     env={'LANG': 'en_US',
                                          'PATH': os.environ['PATH']})

    m = re.findall(r'^Status: active\n', output, re.M)

    return len(m) >= 1


def is_ipv6_ok(soft_fail=False):
    """
    Check if IPv6 support is present and ip6tables functional

    :param soft_fail: If set to True and IPv6 support is broken, then reports
                      that the host doesn't have IPv6 support, otherwise a
                      UFWIPv6Error exception is raised.
    :returns: True if IPv6 is working, False otherwise
    """

    # do we have IPv6 in the machine?
    if os.path.isdir('/proc/sys/net/ipv6'):
        # is ip6tables kernel module loaded?
        if not is_module_loaded('ip6_tables'):
            # ip6tables support isn't complete, let's try to load it
            try:
                modprobe('ip6_tables')
                # great, we can load the module
                return True
            except subprocess.CalledProcessError as ex:
                hookenv.log("Couldn't load ip6_tables module: %s" % ex.output,
                            level="WARN")
                # we are in a world where ip6tables isn't working
                if soft_fail:
                    # so we inform that the machine doesn't have IPv6
                    return False
                else:
                    raise UFWIPv6Error("IPv6 firewall support broken")
        else:
            # the module is present :)
            return True

    else:
        # the system doesn't have IPv6
        return False


def disable_ipv6():
    """
    Disable ufw IPv6 support in /etc/default/ufw
    """
    exit_code = subprocess.call(['sed', '-i', 's/IPV6=.*/IPV6=no/g',
                                 '/etc/default/ufw'])
    if exit_code == 0:
        hookenv.log('IPv6 support in ufw disabled', level='INFO')
    else:
        hookenv.log("Couldn't disable IPv6 support in ufw", level="ERROR")
        raise UFWError("Couldn't disable IPv6 support in ufw")


def enable(soft_fail=False):
    """
    Enable ufw

    :param soft_fail: If set to True silently disables IPv6 support in ufw,
                      otherwise a UFWIPv6Error exception is raised when IP6
                      support is broken.
    :returns: True if ufw is successfully enabled
    """
    if is_enabled():
        return True

    if not is_ipv6_ok(soft_fail):
        disable_ipv6()

    output = subprocess.check_output(['ufw', 'enable'],
                                     universal_newlines=True,
                                     env={'LANG': 'en_US',
                                          'PATH': os.environ['PATH']})

    m = re.findall('^Firewall is active and enabled on system startup\n',
                   output, re.M)
    hookenv.log(output, level='DEBUG')

    if len(m) == 0:
        hookenv.log("ufw couldn't be enabled", level='WARN')
        return False
    else:
        hookenv.log("ufw enabled", level='INFO')
        return True


def disable():
    """
    Disable ufw

    :returns: True if ufw is successfully disabled
    """
    if not is_enabled():
        return True

    output = subprocess.check_output(['ufw', 'disable'],
                                     universal_newlines=True,
                                     env={'LANG': 'en_US',
                                          'PATH': os.environ['PATH']})

    m = re.findall(r'^Firewall stopped and disabled on system startup\n',
                   output, re.M)
    hookenv.log(output, level='DEBUG')

    if len(m) == 0:
        hookenv.log("ufw couldn't be disabled", level='WARN')
        return False
    else:
        hookenv.log("ufw disabled", level='INFO')
        return True


def default_policy(policy='deny', direction='incoming'):
    """
    Changes the default policy for traffic `direction`

    :param policy: allow, deny or reject
    :param direction: traffic direction, possible values: incoming, outgoing,
                      routed
    """
    if policy not in ['allow', 'deny', 'reject']:
        raise UFWError(('Unknown policy %s, valid values: '
                        'allow, deny, reject') % policy)

    if direction not in ['incoming', 'outgoing', 'routed']:
        raise UFWError(('Unknown direction %s, valid values: '
                        'incoming, outgoing, routed') % direction)

    output = subprocess.check_output(['ufw', 'default', policy, direction],
                                     universal_newlines=True,
                                     env={'LANG': 'en_US',
                                          'PATH': os.environ['PATH']})
    hookenv.log(output, level='DEBUG')

    m = re.findall("^Default %s policy changed to '%s'\n" % (direction,
                                                             policy),
                   output, re.M)
    if len(m) == 0:
        hookenv.log("ufw couldn't change the default policy to %s for %s"
                    % (policy, direction), level='WARN')
        return False
    else:
        hookenv.log("ufw default policy for %s changed to %s"
                    % (direction, policy), level='INFO')
        return True


def modify_access(src, dst='any', port=None, proto=None, action='allow',
                  index=None):
    """
    Grant access to an address or subnet

    :param src: address (e.g. 192.168.1.234) or subnet
                (e.g. 192.168.1.0/24).
    :param dst: destiny of the connection, if the machine has multiple IPs and
                connections to only one of those have to accepted this is the
                field has to be set.
    :param port: destiny port
    :param proto: protocol (tcp or udp)
    :param action: `allow` or `delete`
    :param index: if different from None the rule is inserted at the given
                  `index`.
    """
    if not is_enabled():
        hookenv.log('ufw is disabled, skipping modify_access()', level='WARN')
        return

    if action == 'delete':
        cmd = ['ufw', 'delete', 'allow']
    elif index is not None:
        cmd = ['ufw', 'insert', str(index), action]
    else:
        cmd = ['ufw', action]

    if src is not None:
        cmd += ['from', src]

    if dst is not None:
        cmd += ['to', dst]

    if port is not None:
        cmd += ['port', str(port)]

    if proto is not None:
        cmd += ['proto', proto]

    hookenv.log('ufw {}: {}'.format(action, ' '.join(cmd)), level='DEBUG')
    p = subprocess.Popen(cmd, stdout=subprocess.PIPE)
    (stdout, stderr) = p.communicate()

    hookenv.log(stdout, level='INFO')

    if p.returncode != 0:
        hookenv.log(stderr, level='ERROR')
        hookenv.log('Error running: {}, exit code: {}'.format(' '.join(cmd),
                                                              p.returncode),
                    level='ERROR')


def grant_access(src, dst='any', port=None, proto=None, index=None):
    """
    Grant access to an address or subnet

    :param src: address (e.g. 192.168.1.234) or subnet
                (e.g. 192.168.1.0/24).
    :param dst: destiny of the connection, if the machine has multiple IPs and
                connections to only one of those have to accepted this is the
                field has to be set.
    :param port: destiny port
    :param proto: protocol (tcp or udp)
    :param index: if different from None the rule is inserted at the given
                  `index`.
    """
    return modify_access(src, dst=dst, port=port, proto=proto, action='allow',
                         index=index)


def revoke_access(src, dst='any', port=None, proto=None):
    """
    Revoke access to an address or subnet

    :param src: address (e.g. 192.168.1.234) or subnet
                (e.g. 192.168.1.0/24).
    :param dst: destiny of the connection, if the machine has multiple IPs and
                connections to only one of those have to accepted this is the
                field has to be set.
    :param port: destiny port
    :param proto: protocol (tcp or udp)
    """
    return modify_access(src, dst=dst, port=port, proto=proto, action='delete')


def service(name, action):
    """
    Open/close access to a service

    :param name: could be a service name defined in `/etc/services` or a port
                 number.
    :param action: `open` or `close`
    """
    if action == 'open':
        subprocess.check_output(['ufw', 'allow', str(name)],
                                universal_newlines=True)
    elif action == 'close':
        subprocess.check_output(['ufw', 'delete', 'allow', str(name)],
                                universal_newlines=True)
    else:
        raise UFWError(("'{}' not supported, use 'allow' "
                        "or 'delete'").format(action))
