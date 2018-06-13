# Copyright 2016 Canonical Ltd.
#
# This file is part of the PostgreSQL Client Interface for Juju charms.reactive
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but
# WITHOUT ANY WARRANTY; without even the implied warranties of
# MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
# PURPOSE.  See the GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

from collections import UserDict
import os.path
import sys
import unittest
from unittest.mock import patch

sys.path.append(os.path.join(os.path.dirname(__file__), os.pardir))

import requires
from requires import ConnectionString


class TestConnectionStringConstructor(unittest.TestCase):
    def setUp(self):
        self.reldata = UserDict({'allowed-units': 'client/0 client/9 client/8',
                                 'host': '10.9.8.7',
                                 'port': '5433',
                                 'database': 'mydata',
                                 'user': 'mememe',
                                 'password': 'secret'})
        self.reldata.relname = 'relname'
        self.reldata.relid = 'relname:42'

        local_unit = self.patch('charmhelpers.core.hookenv.local_unit')
        local_unit.return_value = 'client/9'

        rels = self.patch('charmhelpers.context.Relations')
        rels()['relname']['relname:42'].local = {'database': 'mydata'}

    def patch(self, dotpath):
        patcher = patch(dotpath, autospec=True)
        mock = patcher.start()
        self.addCleanup(patcher.stop)
        return mock

    def test_normal(self):
        conn_str = requires._cs(self.reldata)
        self.assertIsNotNone(conn_str)
        self.assertIsInstance(conn_str, ConnectionString)
        self.assertEqual(conn_str,
                         'dbname=mydata host=10.9.8.7 password=secret '
                         'port=5433 user=mememe')

    def test_missing_attr(self):
        del self.reldata['port']
        self.assertIsNone(requires._cs(self.reldata))

    def test_incorrect_database(self):
        self.reldata['database'] = 'notherdb'
        self.assertIsNone(requires._cs(self.reldata))

    def test_unauthorized(self):
        self.reldata['allowed-units'] = 'client/90'
        self.assertIsNone(requires._cs(self.reldata))

    def test_no_auth(self):
        del self.reldata['allowed-units']
        self.assertIsNone(requires._cs(self.reldata))
