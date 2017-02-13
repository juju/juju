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

import six
import subprocess

from charmhelpers.core.hookenv import (
    log,
    WARNING,
)
from charmhelpers.contrib.hardening.audits.file import (
    FilePermissionAudit,
    DirectoryPermissionAudit,
    TemplatedFile,
)
from charmhelpers.contrib.hardening.mysql import TEMPLATES_DIR
from charmhelpers.contrib.hardening import utils


def get_audits():
    """Get MySQL hardening config audits.

    :returns:  dictionary of audits
    """
    if subprocess.call(['which', 'mysql'], stdout=subprocess.PIPE) != 0:
        log("MySQL does not appear to be installed on this node - "
            "skipping mysql hardening", level=WARNING)
        return []

    settings = utils.get_settings('mysql')
    hardening_settings = settings['hardening']
    my_cnf = hardening_settings['mysql-conf']

    audits = [
        FilePermissionAudit(paths=[my_cnf], user='root',
                            group='root', mode=0o0600),

        TemplatedFile(hardening_settings['hardening-conf'],
                      MySQLConfContext(),
                      TEMPLATES_DIR,
                      mode=0o0750,
                      user='mysql',
                      group='root',
                      service_actions=[{'service': 'mysql',
                                        'actions': ['restart']}]),

        # MySQL and Percona charms do not allow configuration of the
        # data directory, so use the default.
        DirectoryPermissionAudit('/var/lib/mysql',
                                 user='mysql',
                                 group='mysql',
                                 recursive=False,
                                 mode=0o755),

        DirectoryPermissionAudit('/etc/mysql',
                                 user='root',
                                 group='root',
                                 recursive=False,
                                 mode=0o700),
    ]

    return audits


class MySQLConfContext(object):
    """Defines the set of key/value pairs to set in a mysql config file.

    This context, when called, will return a dictionary containing the
    key/value pairs of setting to specify in the
    /etc/mysql/conf.d/hardening.cnf file.
    """
    def __call__(self):
        settings = utils.get_settings('mysql')
        # Translate for python3
        return {'mysql_settings':
                [(k, v) for k, v in six.iteritems(settings['security'])]}
