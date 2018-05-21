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

import os.path
import sys
import unittest

sys.path.append(os.path.join(os.path.dirname(__file__), os.pardir))

from requires import ConnectionString


class TestConnectionString(unittest.TestCase):
    def setUp(self):
        self.params = dict(user='mememe',
                           password='secret',
                           host='10.9.8.7',
                           port='5432',
                           dbname='mydata',
                           sslmode='require',
                           connect_timeout='10')

    def test_libpq_str(self):
        conn_str = ConnectionString(**self.params)

        # It is a real str, usable from C extensions.
        self.assertTrue(isinstance(conn_str, str))

        self.assertEqual(conn_str,
                         'connect_timeout=10 '
                         'dbname=mydata '
                         'host=10.9.8.7 '
                         'password=secret '
                         'port=5432 '
                         'sslmode=require '
                         'user=mememe')

    def test_libpq_empty(self):
        conn_str = ConnectionString()
        self.assertEqual(conn_str, '')  # Default everything

    def test_attrs(self):
        conn_str = ConnectionString(**self.params)
        for k, v in self.params.items():
            self.assertEqual(getattr(conn_str, k), v)

    def test_quoting(self):
        params = dict(backslash=r"Back\slash",
                      quote="quote's")
        conn_str = ConnectionString(**params)
        self.assertEqual(conn_str,
                         r"backslash=Back\\slash "
                         r"quote=quote\'s")

    def test_keys(self):
        conn_str = ConnectionString(**self.params)
        self.assertListEqual(sorted(conn_str.keys()),
                             sorted(self.params.keys()))

    def test_items(self):
        conn_str = ConnectionString(**self.params)
        self.assertListEqual(sorted(conn_str.items()),
                             sorted(self.params.items()))

    def test_values(self):
        conn_str = ConnectionString(**self.params)
        self.assertListEqual(sorted(conn_str.values()),
                             sorted(self.params.values()))

    def test_getitem(self):
        conn_str = ConnectionString(**self.params)
        for k, v in self.params.items():
            self.assertEqual(conn_str[k], v)

    def test_uri(self):
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@10.9.8.7:5432/mydata'
                         '?connect_timeout=10&sslmode=require')

    def test_uri_no_user(self):
        del self.params['user']
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://10.9.8.7:5432/mydata'
                         '?connect_timeout=10&sslmode=require')

    def test_uri_no_password(self):
        del self.params['password']
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe@10.9.8.7:5432/mydata'
                         '?connect_timeout=10&sslmode=require')

    def test_uri_no_host(self):
        del self.params['host']
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@:5432/mydata'
                         '?connect_timeout=10&sslmode=require')

    def test_uri_no_port(self):
        del self.params['port']
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@10.9.8.7/mydata'
                         '?connect_timeout=10&sslmode=require')

    def test_uri_no_dbname(self):
        del self.params['dbname']
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@10.9.8.7:5432'
                         '?connect_timeout=10&sslmode=require')

    def test_uri_no_extras(self):
        del self.params['connect_timeout']
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@10.9.8.7:5432/mydata'
                         '?sslmode=require')
        del self.params['sslmode']
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@10.9.8.7:5432/mydata')

    def test_uri_empty(self):
        conn_str = ConnectionString()
        self.assertEqual(conn_str.uri, 'postgresql://')  # Default everything

    def test_uri_quoting(self):
        params = dict(user='fred?',
                      password="secret's",
                      sslmode="&")
        conn_str = ConnectionString(**params)
        self.assertEqual(conn_str.uri,
                         "postgresql://fred%3F:secret%27s@?sslmode=%26")

    def test_uri_ipv6(self):
        self.params['host'] = '2001:db8::1234'
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@[2001:db8::1234]:5432'
                         '/mydata?connect_timeout=10&sslmode=require')

    def test_uri_hostname(self):
        self.params['host'] = 'hname'
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@hname:5432/mydata'
                         '?connect_timeout=10&sslmode=require')

    def test_uri_hostaddr(self):
        # hostaddr is prefered over host in the URI, so it behaves the
        # same as the libpq connection string. We could set host to a
        # name and hostaddr to the ip addres and it should work
        # (but we won't, because there will be tools that will construct
        # their own connection strings and don't know about hostaddr
        self.params['host'] = 'unit_0'
        self.params['hostaddr'] = '10.0.1.2'
        conn_str = ConnectionString(**self.params)
        self.assertEqual(conn_str.uri,
                         'postgresql://mememe:secret@10.0.1.2:5432/mydata'
                         '?connect_timeout=10&sslmode=require')
