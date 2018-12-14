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
import re
import six
import subprocess


from charmhelpers.core.hookenv import (
    log,
    INFO,
)
from charmhelpers.contrib.hardening.audits.file import (
    FilePermissionAudit,
    DirectoryPermissionAudit,
    NoReadWriteForOther,
    TemplatedFile,
    DeletedFile
)
from charmhelpers.contrib.hardening.audits.apache import DisabledModuleAudit
from charmhelpers.contrib.hardening.apache import TEMPLATES_DIR
from charmhelpers.contrib.hardening import utils


def get_audits():
    """Get Apache hardening config audits.

    :returns:  dictionary of audits
    """
    if subprocess.call(['which', 'apache2'], stdout=subprocess.PIPE) != 0:
        log("Apache server does not appear to be installed on this node - "
            "skipping apache hardening", level=INFO)
        return []

    context = ApacheConfContext()
    settings = utils.get_settings('apache')
    audits = [
        FilePermissionAudit(paths=os.path.join(
                            settings['common']['apache_dir'], 'apache2.conf'),
                            user='root', group='root', mode=0o0640),

        TemplatedFile(os.path.join(settings['common']['apache_dir'],
                                   'mods-available/alias.conf'),
                      context,
                      TEMPLATES_DIR,
                      mode=0o0640,
                      user='root',
                      service_actions=[{'service': 'apache2',
                                        'actions': ['restart']}]),

        TemplatedFile(os.path.join(settings['common']['apache_dir'],
                                   'conf-enabled/99-hardening.conf'),
                      context,
                      TEMPLATES_DIR,
                      mode=0o0640,
                      user='root',
                      service_actions=[{'service': 'apache2',
                                        'actions': ['restart']}]),

        DirectoryPermissionAudit(settings['common']['apache_dir'],
                                 user='root',
                                 group='root',
                                 mode=0o0750),

        DisabledModuleAudit(settings['hardening']['modules_to_disable']),

        NoReadWriteForOther(settings['common']['apache_dir']),

        DeletedFile(['/var/www/html/index.html'])
    ]

    return audits


class ApacheConfContext(object):
    """Defines the set of key/value pairs to set in a apache config file.

    This context, when called, will return a dictionary containing the
    key/value pairs of setting to specify in the
    /etc/apache/conf-enabled/hardening.conf file.
    """
    def __call__(self):
        settings = utils.get_settings('apache')
        ctxt = settings['hardening']

        out = subprocess.check_output(['apache2', '-v'])
        if six.PY3:
            out = out.decode('utf-8')
        ctxt['apache_version'] = re.search(r'.+version: Apache/(.+?)\s.+',
                                           out).group(1)
        ctxt['apache_icondir'] = '/usr/share/apache2/icons/'
        return ctxt
