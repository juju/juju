#!/usr/bin/python
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from charms.reactive import Endpoint
from charms.reactive import when, when_not
from charms.reactive import set_flag, clear_flag
from charms.reactive import data_changed


class MySQLClient(Endpoint):
    @when('endpoint.{endpoint_name}.joined')
    def _handle_joined(self):
        # translate automatic internal joined flag to published connected flag
        set_flag(self.expand_name('{endpoint_name}.connected'))

    @when('endpoint.{endpoint_name}.changed')
    def _handle_changed(self):
        set_flag(self.expand_name('{endpoint_name}.connected'))
        if self.connection_string():
            set_flag(self.expand_name('{endpoint_name}.available'))
            data_key = self.expand_name('endpoint.{endpoint_name}.data')
            if data_changed(data_key, self.connection_string()):
                set_flag(self.expand_name('{endpoint_name}.changed'))

    @when_not('endpoint.{endpoint_name}.joined')
    def _handle_broken(self):
        clear_flag(self.expand_name('{endpoint_name}.connected'))
        clear_flag(self.expand_name('{endpoint_name}.available'))

    def connection_string(self):
        """
        Get the connection string, if available, or None.

        The connection string will be in the format::

            'host={host} port={port} dbname={database} '
            'user={user} password={password}'
        """
        data = {
            'host': self.host(),
            'port': self.port(),
            'database': self.database(),
            'user': self.user(),
            'password': self.password(),
        }
        if all(data.values()):
            return str.format(
                'host={host} port={port} dbname={database} '
                'user={user} password={password}',
                **data)
        return None

    def database(self):
        """
        Return the name of the provided database.
        """
        return self.all_joined_units.received_raw['database']

    def host(self):
        """
        Return the host for the provided database.
        """
        return self.all_joined_units.received_raw['host']

    def port(self):
        """
        Return the port the provided database.

        If not available, returns the default port of 3306.
        """
        return self.all_joined_units.received_raw.get('port', 3306)

    def user(self):
        """
        Return the username for the provided database.
        """
        return self.all_joined_units.received_raw['user']

    def password(self):
        """
        Return the password for the provided database.
        """
        return self.all_joined_units.received_raw['password']
