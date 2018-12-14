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

from subprocess import (
    check_output,
    CalledProcessError,
)

from charmhelpers.core.hookenv import (
    log,
    DEBUG,
    ERROR,
)
from charmhelpers.fetch import (
    apt_install,
    apt_purge,
    apt_update,
)
from charmhelpers.contrib.hardening.audits.file import (
    TemplatedFile,
    DeletedFile,
)
from charmhelpers.contrib.hardening import utils
from charmhelpers.contrib.hardening.host import TEMPLATES_DIR


def get_audits():
    """Get OS hardening PAM authentication audits.

    :returns:  dictionary of audits
    """
    audits = []

    settings = utils.get_settings('os')

    if settings['auth']['pam_passwdqc_enable']:
        audits.append(PasswdqcPAM('/etc/passwdqc.conf'))

    if settings['auth']['retries']:
        audits.append(Tally2PAM('/usr/share/pam-configs/tally2'))
    else:
        audits.append(DeletedFile('/usr/share/pam-configs/tally2'))

    return audits


class PasswdqcPAMContext(object):

    def __call__(self):
        ctxt = {}
        settings = utils.get_settings('os')

        ctxt['auth_pam_passwdqc_options'] = \
            settings['auth']['pam_passwdqc_options']

        return ctxt


class PasswdqcPAM(TemplatedFile):
    """The PAM Audit verifies the linux PAM settings."""
    def __init__(self, path):
        super(PasswdqcPAM, self).__init__(path=path,
                                          template_dir=TEMPLATES_DIR,
                                          context=PasswdqcPAMContext(),
                                          user='root',
                                          group='root',
                                          mode=0o0640)

    def pre_write(self):
        # Always remove?
        for pkg in ['libpam-ccreds', 'libpam-cracklib']:
            log("Purging package '%s'" % pkg, level=DEBUG),
            apt_purge(pkg)

        apt_update(fatal=True)
        for pkg in ['libpam-passwdqc']:
            log("Installing package '%s'" % pkg, level=DEBUG),
            apt_install(pkg)

    def post_write(self):
        """Updates the PAM configuration after the file has been written"""
        try:
            check_output(['pam-auth-update', '--package'])
        except CalledProcessError as e:
            log('Error calling pam-auth-update: %s' % e, level=ERROR)


class Tally2PAMContext(object):

    def __call__(self):
        ctxt = {}
        settings = utils.get_settings('os')

        ctxt['auth_lockout_time'] = settings['auth']['lockout_time']
        ctxt['auth_retries'] = settings['auth']['retries']

        return ctxt


class Tally2PAM(TemplatedFile):
    """The PAM Audit verifies the linux PAM settings."""
    def __init__(self, path):
        super(Tally2PAM, self).__init__(path=path,
                                        template_dir=TEMPLATES_DIR,
                                        context=Tally2PAMContext(),
                                        user='root',
                                        group='root',
                                        mode=0o0640)

    def pre_write(self):
        # Always remove?
        apt_purge('libpam-ccreds')
        apt_update(fatal=True)
        apt_install('libpam-modules')

    def post_write(self):
        """Updates the PAM configuration after the file has been written"""
        try:
            check_output(['pam-auth-update', '--package'])
        except CalledProcessError as e:
            log('Error calling pam-auth-update: %s' % e, level=ERROR)
