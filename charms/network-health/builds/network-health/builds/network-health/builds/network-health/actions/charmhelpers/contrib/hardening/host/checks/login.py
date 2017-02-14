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

from six import string_types

from charmhelpers.contrib.hardening.audits.file import TemplatedFile
from charmhelpers.contrib.hardening.host import TEMPLATES_DIR
from charmhelpers.contrib.hardening import utils


def get_audits():
    """Get OS hardening login.defs audits.

    :returns:  dictionary of audits
    """
    audits = [TemplatedFile('/etc/login.defs', LoginContext(),
                            template_dir=TEMPLATES_DIR,
                            user='root', group='root', mode=0o0444)]
    return audits


class LoginContext(object):

    def __call__(self):
        settings = utils.get_settings('os')

        # Octal numbers in yaml end up being turned into decimal,
        # so check if the umask is entered as a string (e.g. '027')
        # or as an octal umask as we know it (e.g. 002). If its not
        # a string assume it to be octal and turn it into an octal
        # string.
        umask = settings['environment']['umask']
        if not isinstance(umask, string_types):
            umask = '%s' % oct(umask)

        ctxt = {
            'additional_user_paths':
            settings['environment']['extra_user_paths'],
            'umask': umask,
            'pwd_max_age': settings['auth']['pw_max_age'],
            'pwd_min_age': settings['auth']['pw_min_age'],
            'uid_min': settings['auth']['uid_min'],
            'sys_uid_min': settings['auth']['sys_uid_min'],
            'sys_uid_max': settings['auth']['sys_uid_max'],
            'gid_min': settings['auth']['gid_min'],
            'sys_gid_min': settings['auth']['sys_gid_min'],
            'sys_gid_max': settings['auth']['sys_gid_max'],
            'login_retries': settings['auth']['retries'],
            'login_timeout': settings['auth']['timeout'],
            'chfn_restrict': settings['auth']['chfn_restrict'],
            'allow_login_without_home': settings['auth']['allow_homeless']
        }

        return ctxt
