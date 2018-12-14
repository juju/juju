# Copyright 2016-2018 Canonical Ltd.
#
# This file is part of the PostgreSQL Client Interface for Juju charms.reactive
#
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

from charms import reactive
from charms.reactive import when, when_not


class PostgreSQLServer(reactive.Endpoint):
    """
    PostgreSQL partial server side interface.
    """
    @when('endpoint.{endpoint_name}.joined')
    @when_not('{endpoint_name}.connected')
    def joined(self):
        reactive.set_flag(self.expand_name('{endpoint_name}.connected'))

    @when('{endpoint_name}.connected')
    @when_not('endpoint.{endpoint_name}.joined')
    def departed(self):
        reactive.clear_flag(self.expand_name('{endpoint_name}.connected'))
