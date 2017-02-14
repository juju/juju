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
    """Get OS hardening Secure TTY audits.

    :returns:  dictionary of audits
    """
    audits = []
    audits.append(TemplatedFile('/etc/securetty', SecureTTYContext(),
                                template_dir=TEMPLATES_DIR,
                                mode=0o0400, user='root', group='root'))
    return audits


class SecureTTYContext(object):

    def __call__(self):
        settings = utils.get_settings('os')
        ctxt = {'ttys': settings['auth']['root_ttys']}
        return ctxt
