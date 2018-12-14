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

from charmhelpers.contrib.hardening.audits.file import TemplatedFile
from charmhelpers.contrib.hardening.host import TEMPLATES_DIR
from charmhelpers.contrib.hardening import utils


def get_audits():
    """Get OS hardening profile audits.

    :returns:  dictionary of audits
    """
    audits = []

    settings = utils.get_settings('os')
    # If core dumps are not enabled, then don't allow core dumps to be
    # created as they may contain sensitive information.
    if not settings['security']['kernel_enable_core_dump']:
        audits.append(TemplatedFile('/etc/profile.d/pinerolo_profile.sh',
                                    ProfileContext(),
                                    template_dir=TEMPLATES_DIR,
                                    mode=0o0755, user='root', group='root'))
    if settings['security']['ssh_tmout']:
        audits.append(TemplatedFile('/etc/profile.d/99-hardening.sh',
                                    ProfileContext(),
                                    template_dir=TEMPLATES_DIR,
                                    mode=0o0644, user='root', group='root'))
    return audits


class ProfileContext(object):

    def __call__(self):
        settings = utils.get_settings('os')
        ctxt = {'ssh_tmout':
                settings['security']['ssh_tmout']}
        return ctxt
