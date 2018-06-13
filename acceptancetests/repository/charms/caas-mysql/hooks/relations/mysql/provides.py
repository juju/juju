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
from charms.reactive import set_flag, clear_flag
from charms.reactive import when, when_not


class MySQL(Endpoint):
    @when('endpoint.{endpoint_name}.joined')
    def _handle_joined(self):
        # translate the automatic internal joined flag into a DB requested flag
        set_flag(self.expand_name('{endpoint_name}.database.requested'))

    @when_not('endpoint.{endpoint_name}.joined')
    def _handle_broken(self):
        # if these were requested but not yet fulfilled, cancel the request
        clear_flag(self.expand_name('{endpoint_name}.database.requested'))

    def database_requests(self):
        """
        Return a list of requests for databases.

        This returns a mapping of request IDs to application names.

        Example usage::

            @when('clients.database.requested')
            def create_dbs():
                mysql = endpoint_from_flag('clients.database.requested')
                for request, application in mysql.database_requests().items():
                    db_name = generate_dbname(application)
                    host, port, user, password = create_database(db_name)
                    mysql.provide_database(request, db_name,
                                           host, port, user, password)
                mysql.mark_complete()
        """
        return {relation.relation_id: relation.application_name
                for relation in self.relations}

    def provide_database(self, request_id, database_name,
                         host, port, user, password):
        """
        Provide a database to a requesting application.

        :param str request_id: The ID for the database request, as
            returned by :meth:`~provides.MySQL.requested_databases`.
        :param str database_name: The name of the database being provided.
        :param str host: The host where the database can be reached (e.g.,
            the charm's private or public-address).
        :param int port: The port where the database can be reached.
        :param str user: The username to be used to access the database.
        :param str password: The password to be used to access the database.
        """
        relation = self.relations[request_id]
        relation.to_publish_raw.update({
            'database': database_name,
            'host': host,
            'port': port,
            'user': user,
            'password': password,
        })

    def mark_complete(self):
        clear_flag(self.expand_name('{endpoint_name}.database.requested'))
