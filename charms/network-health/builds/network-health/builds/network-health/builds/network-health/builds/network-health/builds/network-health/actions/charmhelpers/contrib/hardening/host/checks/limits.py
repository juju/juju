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

from charmhelpers.contrib.hardening.audits.file import (
    DirectoryPermissionAudit,
    TemplatedFile,
)
from charmhelpers.contrib.hardening.host import TEMPLATES_DIR
from charmhelpers.contrib.hardening import utils


def get_audits():
    """Get OS hardening security limits audits.

    :returns:  dictionary of audits
    """
    audits = []
    settings = utils.get_settings('os')

    # Ensure that the /etc/security/limits.d directory is only writable
    # by the root user, but others can execute and read.
    audits.append(DirectoryPermissionAudit('/etc/security/limits.d',
                                           user='root', group='root',
                                           mode=0o755))

    # If core dumps are not enabled, then don't allow core dumps to be
    # created as they may contain sensitive information.
    if not settings['security']['kernel_enable_core_dump']:
        audits.append(TemplatedFile('/etc/security/limits.d/10.hardcore.conf',
                                    SecurityLimitsContext(),
                                    template_dir=TEMPLATES_DIR,
                                    user='root', group='root', mode=0o0440))
    return audits


class SecurityLimitsContext(object):

    def __call__(self):
        settings = utils.get_settings('os')
        ctxt = {'disable_core_dump':
                not settings['security']['kernel_enable_core_dump']}
        return ctxt
