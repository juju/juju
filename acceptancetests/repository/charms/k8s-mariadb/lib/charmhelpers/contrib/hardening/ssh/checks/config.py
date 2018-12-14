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

import os

from charmhelpers.contrib.network.ip import (
    get_address_in_network,
    get_iface_addr,
    is_ip,
)
from charmhelpers.core.hookenv import (
    log,
    DEBUG,
)
from charmhelpers.fetch import (
    apt_install,
    apt_update,
)
from charmhelpers.core.host import (
    lsb_release,
    CompareHostReleases,
)
from charmhelpers.contrib.hardening.audits.file import (
    TemplatedFile,
    FileContentAudit,
)
from charmhelpers.contrib.hardening.ssh import TEMPLATES_DIR
from charmhelpers.contrib.hardening import utils


def get_audits():
    """Get SSH hardening config audits.

    :returns:  dictionary of audits
    """
    audits = [SSHConfig(), SSHDConfig(), SSHConfigFileContentAudit(),
              SSHDConfigFileContentAudit()]
    return audits


class SSHConfigContext(object):

    type = 'client'

    def get_macs(self, allow_weak_mac):
        if allow_weak_mac:
            weak_macs = 'weak'
        else:
            weak_macs = 'default'

        default = 'hmac-sha2-512,hmac-sha2-256,hmac-ripemd160'
        macs = {'default': default,
                'weak': default + ',hmac-sha1'}

        default = ('hmac-sha2-512-etm@openssh.com,'
                   'hmac-sha2-256-etm@openssh.com,'
                   'hmac-ripemd160-etm@openssh.com,umac-128-etm@openssh.com,'
                   'hmac-sha2-512,hmac-sha2-256,hmac-ripemd160')
        macs_66 = {'default': default,
                   'weak': default + ',hmac-sha1'}

        # Use newer ciphers on Ubuntu Trusty and above
        _release = lsb_release()['DISTRIB_CODENAME'].lower()
        if CompareHostReleases(_release) >= 'trusty':
            log("Detected Ubuntu 14.04 or newer, using new macs", level=DEBUG)
            macs = macs_66

        return macs[weak_macs]

    def get_kexs(self, allow_weak_kex):
        if allow_weak_kex:
            weak_kex = 'weak'
        else:
            weak_kex = 'default'

        default = 'diffie-hellman-group-exchange-sha256'
        weak = (default + ',diffie-hellman-group14-sha1,'
                'diffie-hellman-group-exchange-sha1,'
                'diffie-hellman-group1-sha1')
        kex = {'default': default,
               'weak': weak}

        default = ('curve25519-sha256@libssh.org,'
                   'diffie-hellman-group-exchange-sha256')
        weak = (default + ',diffie-hellman-group14-sha1,'
                'diffie-hellman-group-exchange-sha1,'
                'diffie-hellman-group1-sha1')
        kex_66 = {'default': default,
                  'weak': weak}

        # Use newer kex on Ubuntu Trusty and above
        _release = lsb_release()['DISTRIB_CODENAME'].lower()
        if CompareHostReleases(_release) >= 'trusty':
            log('Detected Ubuntu 14.04 or newer, using new key exchange '
                'algorithms', level=DEBUG)
            kex = kex_66

        return kex[weak_kex]

    def get_ciphers(self, cbc_required):
        if cbc_required:
            weak_ciphers = 'weak'
        else:
            weak_ciphers = 'default'

        default = 'aes256-ctr,aes192-ctr,aes128-ctr'
        cipher = {'default': default,
                  'weak': default + 'aes256-cbc,aes192-cbc,aes128-cbc'}

        default = ('chacha20-poly1305@openssh.com,aes256-gcm@openssh.com,'
                   'aes128-gcm@openssh.com,aes256-ctr,aes192-ctr,aes128-ctr')
        ciphers_66 = {'default': default,
                      'weak': default + ',aes256-cbc,aes192-cbc,aes128-cbc'}

        # Use newer ciphers on ubuntu Trusty and above
        _release = lsb_release()['DISTRIB_CODENAME'].lower()
        if CompareHostReleases(_release) >= 'trusty':
            log('Detected Ubuntu 14.04 or newer, using new ciphers',
                level=DEBUG)
            cipher = ciphers_66

        return cipher[weak_ciphers]

    def get_listening(self, listen=['0.0.0.0']):
        """Returns a list of addresses SSH can list on

        Turns input into a sensible list of IPs SSH can listen on. Input
        must be a python list of interface names, IPs and/or CIDRs.

        :param listen: list of IPs, CIDRs, interface names

        :returns: list of IPs available on the host
        """
        if listen == ['0.0.0.0']:
            return listen

        value = []
        for network in listen:
            try:
                ip = get_address_in_network(network=network, fatal=True)
            except ValueError:
                if is_ip(network):
                    ip = network
                else:
                    try:
                        ip = get_iface_addr(iface=network, fatal=False)[0]
                    except IndexError:
                        continue
            value.append(ip)
        if value == []:
            return ['0.0.0.0']
        return value

    def __call__(self):
        settings = utils.get_settings('ssh')
        if settings['common']['network_ipv6_enable']:
            addr_family = 'any'
        else:
            addr_family = 'inet'

        ctxt = {
            'addr_family': addr_family,
            'remote_hosts': settings['common']['remote_hosts'],
            'password_auth_allowed':
            settings['client']['password_authentication'],
            'ports': settings['common']['ports'],
            'ciphers': self.get_ciphers(settings['client']['cbc_required']),
            'macs': self.get_macs(settings['client']['weak_hmac']),
            'kexs': self.get_kexs(settings['client']['weak_kex']),
            'roaming': settings['client']['roaming'],
        }
        return ctxt


class SSHConfig(TemplatedFile):
    def __init__(self):
        path = '/etc/ssh/ssh_config'
        super(SSHConfig, self).__init__(path=path,
                                        template_dir=TEMPLATES_DIR,
                                        context=SSHConfigContext(),
                                        user='root',
                                        group='root',
                                        mode=0o0644)

    def pre_write(self):
        settings = utils.get_settings('ssh')
        apt_update(fatal=True)
        apt_install(settings['client']['package'])
        if not os.path.exists('/etc/ssh'):
            os.makedir('/etc/ssh')
            # NOTE: don't recurse
            utils.ensure_permissions('/etc/ssh', 'root', 'root', 0o0755,
                                     maxdepth=0)

    def post_write(self):
        # NOTE: don't recurse
        utils.ensure_permissions('/etc/ssh', 'root', 'root', 0o0755,
                                 maxdepth=0)


class SSHDConfigContext(SSHConfigContext):

    type = 'server'

    def __call__(self):
        settings = utils.get_settings('ssh')
        if settings['common']['network_ipv6_enable']:
            addr_family = 'any'
        else:
            addr_family = 'inet'

        ctxt = {
            'ssh_ip': self.get_listening(settings['server']['listen_to']),
            'password_auth_allowed':
            settings['server']['password_authentication'],
            'ports': settings['common']['ports'],
            'addr_family': addr_family,
            'ciphers': self.get_ciphers(settings['server']['cbc_required']),
            'macs': self.get_macs(settings['server']['weak_hmac']),
            'kexs': self.get_kexs(settings['server']['weak_kex']),
            'host_key_files': settings['server']['host_key_files'],
            'allow_root_with_key': settings['server']['allow_root_with_key'],
            'password_authentication':
            settings['server']['password_authentication'],
            'use_priv_sep': settings['server']['use_privilege_separation'],
            'use_pam': settings['server']['use_pam'],
            'allow_x11_forwarding': settings['server']['allow_x11_forwarding'],
            'print_motd': settings['server']['print_motd'],
            'print_last_log': settings['server']['print_last_log'],
            'client_alive_interval':
            settings['server']['alive_interval'],
            'client_alive_count': settings['server']['alive_count'],
            'allow_tcp_forwarding': settings['server']['allow_tcp_forwarding'],
            'allow_agent_forwarding':
            settings['server']['allow_agent_forwarding'],
            'deny_users': settings['server']['deny_users'],
            'allow_users': settings['server']['allow_users'],
            'deny_groups': settings['server']['deny_groups'],
            'allow_groups': settings['server']['allow_groups'],
            'use_dns': settings['server']['use_dns'],
            'sftp_enable': settings['server']['sftp_enable'],
            'sftp_group': settings['server']['sftp_group'],
            'sftp_chroot': settings['server']['sftp_chroot'],
            'max_auth_tries': settings['server']['max_auth_tries'],
            'max_sessions': settings['server']['max_sessions'],
        }
        return ctxt


class SSHDConfig(TemplatedFile):
    def __init__(self):
        path = '/etc/ssh/sshd_config'
        super(SSHDConfig, self).__init__(path=path,
                                         template_dir=TEMPLATES_DIR,
                                         context=SSHDConfigContext(),
                                         user='root',
                                         group='root',
                                         mode=0o0600,
                                         service_actions=[{'service': 'ssh',
                                                           'actions':
                                                           ['restart']}])

    def pre_write(self):
        settings = utils.get_settings('ssh')
        apt_update(fatal=True)
        apt_install(settings['server']['package'])
        if not os.path.exists('/etc/ssh'):
            os.makedir('/etc/ssh')
            # NOTE: don't recurse
            utils.ensure_permissions('/etc/ssh', 'root', 'root', 0o0755,
                                     maxdepth=0)

    def post_write(self):
        # NOTE: don't recurse
        utils.ensure_permissions('/etc/ssh', 'root', 'root', 0o0755,
                                 maxdepth=0)


class SSHConfigFileContentAudit(FileContentAudit):
    def __init__(self):
        self.path = '/etc/ssh/ssh_config'
        super(SSHConfigFileContentAudit, self).__init__(self.path, {})

    def is_compliant(self, *args, **kwargs):
        self.pass_cases = []
        self.fail_cases = []
        settings = utils.get_settings('ssh')

        _release = lsb_release()['DISTRIB_CODENAME'].lower()
        if CompareHostReleases(_release) >= 'trusty':
            if not settings['server']['weak_hmac']:
                self.pass_cases.append(r'^MACs.+,hmac-ripemd160$')
            else:
                self.pass_cases.append(r'^MACs.+,hmac-sha1$')

            if settings['server']['weak_kex']:
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha256[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group14-sha1[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha1[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group1-sha1[,\s]?')  # noqa
            else:
                self.pass_cases.append(r'^KexAlgorithms.+,diffie-hellman-group-exchange-sha256$')  # noqa
                self.fail_cases.append(r'^KexAlgorithms.*diffie-hellman-group14-sha1[,\s]?')  # noqa

            if settings['server']['cbc_required']:
                self.pass_cases.append(r'^Ciphers\s.*-cbc[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes128-ctr[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes192-ctr[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes256-ctr[,\s]?')
            else:
                self.fail_cases.append(r'^Ciphers\s.*-cbc[,\s]?')
                self.pass_cases.append(r'^Ciphers\schacha20-poly1305@openssh.com,.+')  # noqa
                self.pass_cases.append(r'^Ciphers\s.*aes128-ctr$')
                self.pass_cases.append(r'^Ciphers\s.*aes192-ctr[,\s]?')
                self.pass_cases.append(r'^Ciphers\s.*aes256-ctr[,\s]?')
        else:
            if not settings['client']['weak_hmac']:
                self.fail_cases.append(r'^MACs.+,hmac-sha1$')
            else:
                self.pass_cases.append(r'^MACs.+,hmac-sha1$')

            if settings['client']['weak_kex']:
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha256[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group14-sha1[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha1[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group1-sha1[,\s]?')  # noqa
            else:
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha256$')  # noqa
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group14-sha1[,\s]?')  # noqa
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha1[,\s]?')  # noqa
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group1-sha1[,\s]?')  # noqa

            if settings['client']['cbc_required']:
                self.pass_cases.append(r'^Ciphers\s.*-cbc[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes128-ctr[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes192-ctr[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes256-ctr[,\s]?')
            else:
                self.fail_cases.append(r'^Ciphers\s.*-cbc[,\s]?')
                self.pass_cases.append(r'^Ciphers\s.*aes128-ctr[,\s]?')
                self.pass_cases.append(r'^Ciphers\s.*aes192-ctr[,\s]?')
                self.pass_cases.append(r'^Ciphers\s.*aes256-ctr[,\s]?')

        if settings['client']['roaming']:
            self.pass_cases.append(r'^UseRoaming yes$')
        else:
            self.fail_cases.append(r'^UseRoaming yes$')

        return super(SSHConfigFileContentAudit, self).is_compliant(*args,
                                                                   **kwargs)


class SSHDConfigFileContentAudit(FileContentAudit):
    def __init__(self):
        self.path = '/etc/ssh/sshd_config'
        super(SSHDConfigFileContentAudit, self).__init__(self.path, {})

    def is_compliant(self, *args, **kwargs):
        self.pass_cases = []
        self.fail_cases = []
        settings = utils.get_settings('ssh')

        _release = lsb_release()['DISTRIB_CODENAME'].lower()
        if CompareHostReleases(_release) >= 'trusty':
            if not settings['server']['weak_hmac']:
                self.pass_cases.append(r'^MACs.+,hmac-ripemd160$')
            else:
                self.pass_cases.append(r'^MACs.+,hmac-sha1$')

            if settings['server']['weak_kex']:
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha256[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group14-sha1[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha1[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group1-sha1[,\s]?')  # noqa
            else:
                self.pass_cases.append(r'^KexAlgorithms.+,diffie-hellman-group-exchange-sha256$')  # noqa
                self.fail_cases.append(r'^KexAlgorithms.*diffie-hellman-group14-sha1[,\s]?')  # noqa

            if settings['server']['cbc_required']:
                self.pass_cases.append(r'^Ciphers\s.*-cbc[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes128-ctr[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes192-ctr[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes256-ctr[,\s]?')
            else:
                self.fail_cases.append(r'^Ciphers\s.*-cbc[,\s]?')
                self.pass_cases.append(r'^Ciphers\schacha20-poly1305@openssh.com,.+')  # noqa
                self.pass_cases.append(r'^Ciphers\s.*aes128-ctr$')
                self.pass_cases.append(r'^Ciphers\s.*aes192-ctr[,\s]?')
                self.pass_cases.append(r'^Ciphers\s.*aes256-ctr[,\s]?')
        else:
            if not settings['server']['weak_hmac']:
                self.pass_cases.append(r'^MACs.+,hmac-ripemd160$')
            else:
                self.pass_cases.append(r'^MACs.+,hmac-sha1$')

            if settings['server']['weak_kex']:
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha256[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group14-sha1[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha1[,\s]?')  # noqa
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group1-sha1[,\s]?')  # noqa
            else:
                self.pass_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha256$')  # noqa
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group14-sha1[,\s]?')  # noqa
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group-exchange-sha1[,\s]?')  # noqa
                self.fail_cases.append(r'^KexAlgorithms\sdiffie-hellman-group1-sha1[,\s]?')  # noqa

            if settings['server']['cbc_required']:
                self.pass_cases.append(r'^Ciphers\s.*-cbc[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes128-ctr[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes192-ctr[,\s]?')
                self.fail_cases.append(r'^Ciphers\s.*aes256-ctr[,\s]?')
            else:
                self.fail_cases.append(r'^Ciphers\s.*-cbc[,\s]?')
                self.pass_cases.append(r'^Ciphers\s.*aes128-ctr[,\s]?')
                self.pass_cases.append(r'^Ciphers\s.*aes192-ctr[,\s]?')
                self.pass_cases.append(r'^Ciphers\s.*aes256-ctr[,\s]?')

        if settings['server']['sftp_enable']:
            self.pass_cases.append(r'^Subsystem\ssftp')
        else:
            self.fail_cases.append(r'^Subsystem\ssftp')

        return super(SSHDConfigFileContentAudit, self).is_compliant(*args,
                                                                    **kwargs)
