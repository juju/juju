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

from collections import OrderedDict
import ipaddress
import itertools
import re
import urllib.parse

from charmhelpers.core import hookenv
from charms.reactive import set_flag, clear_flag, Endpoint, when, when_not


# This data structure cannot be in an external library,
# as interfaces have no way to declare dependencies
# (https://github.com/juju/charm-tools/issues/243).
# It also must be defined in this file
# (https://github.com/juju-solutions/charms.reactive/pull/51)
#
class ConnectionString(str):
    """A libpq connection string.

    >>> c = ConnectionString(host='1.2.3.4', dbname='mydb',
    ...                      port=5432, user='anon', password='secret')
    ...
    >>> c
    'host=1.2.3.4 dbname=mydb port=5432 user=anon password=secret
    >>> ConnectionString(str(c), dbname='otherdb')
    'host=1.2.3.4 dbname=otherdb port=5432 user=anon password=secret

    Components may be accessed as attributes.

    >>> c.dbname
    'mydb'
    >>> c.host
    '1.2.3.4'
    >>> c.port
    '5432'

    The standard URI format is also accessible:

    >>> c.uri
    'postgresql://anon:secret@1.2.3.4:5432/mydb'

    """
    def __new__(self, conn_str=None, **kw):

        # Parse libpq key=value style connection string. Components
        # passed by keyword argument override. If the connection string
        # is invalid, some components may be skipped (but in practice,
        # where database and usernames don't contain whitespace,
        # quotes or backslashes, this doesn't happen).
        if conn_str is not None:
            r = re.compile(r"""(?x)
                               (\w+) \s* = \s*
                               (?:
                                 '((?:.|\.)*?)' |
                                 (\S*)
                               )
                               (?=(?:\s|\Z))
                           """)
            for key, v1, v2 in r.findall(conn_str):
                if key not in kw:
                    kw[key] = v1 or v2

        def quote(x):
            q = str(x).replace("\\", "\\\\").replace("'", "\\'")
            q = q.replace('\n', ' ')  # \n is invalid in connection strings
            if ' ' in q:
                q = "'" + q + "'"
            return q

        c = " ".join("{}={}".format(k, quote(v))
                     for k, v in sorted(kw.items())
                     if v)
        c = str.__new__(self, c)

        for k, v in kw.items():
            setattr(c, k, v)

        self._keys = set(kw.keys())

        # Construct the documented PostgreSQL URI for applications
        # that use this format. PostgreSQL docs refer to this as a
        # URI so we do do, even though it meets the requirements the
        # more specific term URL.
        fmt = ['postgresql://']
        d = {k: urllib.parse.quote(v, safe='') for k, v in kw.items() if v}
        if 'user' in d:
            if 'password' in d:
                fmt.append('{user}:{password}@')
            else:
                fmt.append('{user}@')
        if 'host' in kw:
            try:
                hostaddr = ipaddress.ip_address(kw.get('hostaddr') or
                                                kw.get('host'))
                if isinstance(hostaddr, ipaddress.IPv6Address):
                    d['hostaddr'] = '[{}]'.format(hostaddr)
                else:
                    d['hostaddr'] = str(hostaddr)
            except ValueError:
                # Not an IP address, but hopefully a resolvable name.
                d['hostaddr'] = d['host']
            del d['host']
            fmt.append('{hostaddr}')
        if 'port' in d:
            fmt.append(':{port}')
        if 'dbname' in d:
            fmt.append('/{dbname}')
        main_keys = frozenset(['user', 'password',
                               'dbname', 'hostaddr', 'port'])
        extra_fmt = ['{}={{{}}}'.format(extra, extra)
                     for extra in sorted(d.keys()) if extra not in main_keys]
        if extra_fmt:
            fmt.extend(['?', '&'.join(extra_fmt)])
        c.uri = ''.join(fmt).format(**d)

        return c

    host = None
    dbname = None
    port = None
    user = None
    password = None
    uri = None

    def keys(self):
        return iter(self._keys)

    def items(self):
        return {k: self[k] for k in self.keys()}.items()

    def values(self):
        return iter(self[k] for k in self.keys())

    def __getitem__(self, key):
        if isinstance(key, int):
            return super(ConnectionString, self).__getitem__(key)
        try:
            return getattr(self, key)
        except AttributeError:
            raise KeyError(key)


class ConnectionStrings(OrderedDict):
    """Collection of :class:`ConnectionString` for a relation.

    :class:`ConnectionString` may be accessed as a dictionary
    lookup by unit name, or more usefully by the master and
    standbys attributes. Note that the dictionary lookup may
    return None, when the database is not ready for use.
    """
    relname = None
    relid = None

    def __init__(self, relation):
        super(ConnectionStrings, self).__init__()
        self.relname = relation.relation_id.split(':', 1)[0]
        self.relid = relation.relation_id
        self.relation = relation
        for name, unit in relation.joined_units.items():
            self[name] = _cs(unit)

    @property
    def master(self):
        """The :class:`ConnectionString` for the master, or None."""
        if not self._authorized():
            return None

        # New v2 protocol, each unit advertises the master connection.
        for unit in self.relation.joined_units.values():
            master = unit.received_raw.get('master')
            if master:
                return ConnectionString(master)

        # Fallback to v1 protocol.
        masters = [
            name for name, unit in self.relation.joined_units.items()
            if (self[name] and
                unit.received_raw.get('state') in ('master', 'standalone'))]

        if len(masters) == 1:
            return self[masters[0]]  # One, and only one.
        else:
            # None ready, or multiple due to failover in progress.
            return None

    @property
    def standbys(self):
        """list of :class:`ConnectionString` for active hot standbys."""
        if not self._authorized():
            return None

        # New v2 protocol, each unit advertises all standbys.
        for unit in self.relation.joined_units.values():
            if unit.received_raw.get('standbys'):
                return [ConnectionString(s)
                        for s in unit.received_raw['standbys'].splitlines()
                        if s]

        # Fallback to v1 protocol.
        s = []
        for name, unit in self.relation.joined_units.items():
            if unit.received_raw.get('state') == 'hot standby':
                conn_str = self[name]
                if conn_str:
                    s.append(conn_str)
        return s

    @property
    def version(self):
        """PostgreSQL major version (eg. `9.5`)."""
        for unit in self.relation.joined_units.values():
            if unit.received_raw.get('version'):
                return unit.received_raw['version']
        return None

    def _authorized(self):
        for name, unit in self.relation.joined_units.items():
            d = unit.received_raw
            # Ignore new PostgreSQL units that are not yet providing
            # connection details. ie. all remote units that have not
            # yet run their -relation-joined hook and are yet unaware
            # of this client. This prevents authorization 'flapping'
            # when new remote units are added.
            if 'master' not in d and 'standbys' not in d:
                continue

            # If we don't have a connection string for this unit, it
            # isn't ready for us. We should wait until all units are
            # ready.
            if self[name] is None:
                return False

        return True


class PostgreSQLClient(Endpoint):
    """
    PostgreSQL client interface.

    A client may be related to one or more PostgreSQL services.

    In most cases, a charm will only use a single PostgreSQL
    service being related for each relation defined in metadata.yaml
    (so one per relation name). To access the connection strings, use
    the master and standbys attributes::

        @when('productdb.master.available')
        def setup_database(pgsql):
            conn_str = pgsql.master  # A ConnectionString.
            update_db_conf(conn_str)

        @when('productdb.standbys.available')
        def setup_cache_databases(pgsql):
            set_cache_db_list(pgsql.standbys)  # set of ConnectionString.

    In somecases, a relation name may be related to several PostgreSQL
    services. You can also access the ConnectionStrings for a particular
    service by relation id or by iterating over all of them::

        @when('db.master.available')
        def set_dbs(pgsql):
            update_monitored_dbs(cs.master
                                 for cs in pgsql  # ConnectionStrings.
                                 if cs.master)
    """
    def _set_flag(self, flag):
        set_flag(self.expand_name(flag))

    def _clear_flag(self, flag):
        clear_flag(self.expand_name(flag))

    def _toggle_flag(self, flag, is_set):
        if is_set:
            self._set_flag(flag)
        else:
            self._clear_flag(flag)

    def _clear_all_flags(self):
        self._clear_flag('{endpoint_name}.connected')
        self._clear_flag('{endpoint_name}.master.available')
        self._clear_flag('{endpoint_name}.standbys.available')
        self._clear_flag('{endpoint_name}.database.available')

    def _reset_all_flags(self):
        m, s = self.master, self.standbys
        self._toggle_flag('{endpoint_name}.master.available', m)
        self._toggle_flag('{endpoint_name}.standbys.available', s)
        self._toggle_flag('{endpoint_name}.database.available', m or s)

    @when('endpoint.{endpoint_name}.joined')
    def _joined(self):
        self._set_flag('{endpoint_name}.connected')

    @when_not('endpoint.{endpoint_name}.joined')
    def _departed(self):
        self._clear_all_flags()

    @when('endpoint.{endpoint_name}.changed')
    def _changed(self):
        self._reset_all_flags()
        self._clear_flag('endpoint.{endpoint_name}.changed')

    def _set_raw_value(self, key, value, relid=None):
        for relation in self.relations:
            if relid is None or relid == relation.relation_id:
                relation.to_publish_raw[key] = value
                if relid is not None:
                    break
        self._reset_all_flags()

    def set_database(self, dbname, relid=None):
        """Set the database that the named relations connect to.

        The PostgreSQL service will create the database if necessary. It
        will never remove it.

        :param dbname: The database name. If unspecified, the local service
                       name is used.

        :param relid: relation id to send the database name setting to.
                      If unset, the setting is broadcast to all relations
                      sharing the relation name.

        """
        self._set_raw_value('database', dbname, relid)

    def set_roles(self, roles, relid=None):
        """Provide a set of roles to be granted to the database user.

        Granting permissions to roles allows you to grant database
        access to other charms.

        The PostgreSQL service will create the roles if necessary.
        """
        if isinstance(roles, str):
            roles = [roles]
        roles = ','.join(sorted(roles))
        self._set_raw_value('roles', roles, relid)

    def set_extensions(self, extensions, relid=None):
        """Provide a set of extensions to be installed into the database.

        The PostgreSQL service will attempt to install the requested
        extensions into the database. Extensions not bundled with
        PostgreSQL are normally installed onto the PostgreSQL service
        using the `extra_packages` config setting.
        """
        if isinstance(extensions, str):
            extensions = [extensions]
        extensions = ','.join(sorted(extensions))
        self._set_raw_value('extensions', extensions, relid)

    def __getitem__(self, relid):
        """:returns: :class:`ConnectionStrings` for the relation id."""
        for relation in self.relations:
            if relid == relation.relation_id:
                return ConnectionStrings(relid)
        raise KeyError(relid)

    def __iter__(self):
        """:returns: Iterator of :class:`ConnectionStrings` for this
                     endpoint, one per relation id.
        """
        return iter(ConnectionStrings(relation)
                    for relation in self.relations)

    @property
    def master(self):
        ''':class:`ConnectionString` to the master, or None.

        If multiple PostgreSQL services are related using this relation
        name then the first master found is returned.
        '''
        for cs in self:
            if cs.master:
                return cs.master

    @property
    def standbys(self):
        '''Set of class:`ConnectionString` to the read-only hot standbys.

        If multiple PostgreSQL services are related using this relation
        name then all standbys found are returned.
        '''
        stbys = [cs.standbys for cs in self if cs.standbys is not None]
        return set(itertools.chain(*stbys))

    def connection_string(self, unit=None):
        ''':class:`ConnectionString` to the remote unit, or None.

        unit defaults to the active remote unit.

        You should normally use the master or standbys attributes rather
        than this method.

        If the unit is related multiple times using the same relation
        name, the first one found is returned.
        '''
        if unit is None:
            unit = hookenv.remote_unit()

        found = False
        for relation in self.relations:
            if unit not in relation.joined_units:
                continue
            found = True
            conn_str = _cs(relation.joined_units[unit])
            if conn_str:
                return conn_str

        if found:
            return None  # unit found, but not yet ready.

        raise LookupError(unit)  # unit is not related.


def _csplit(s):
    if s:
        for b in s.split(','):
            b = b.strip()
            if b:
                yield b


def _cs(unit):
    reldata = unit.received_raw
    locdata = unit.relation.to_publish_raw

    d = dict(host=reldata.get('host'),
             port=reldata.get('port'),
             dbname=reldata.get('database'),
             user=reldata.get('user'),
             password=reldata.get('password'))
    if not all(d.values()):
        return None

    # Cannot connect if egress subnets have not been authorized.
    allowed_subnets = set(_csplit(reldata.get('allowed-subnets')))
    if allowed_subnets:
        my_egress = set(_csplit(locdata.get('egress-subnets')))
        if not (my_egress <= allowed_subnets):
            return None
    else:
        # If unit name has not been authorized. This is a legacy protocol,
        # deprecated with Juju 2.3 and cross model relation support.
        local_unit = hookenv.local_unit()
        allowed_units = set(_csplit(reldata.get('allowed-units')))
        if local_unit not in allowed_units:
            return None  # Not yet authorized

    if locdata.get('database') and locdata.get('database', '') != reldata.get('database', ''):
        return None  # Requested database does not match yet
    if locdata.get('roles') and locdata.get('roles', '') != reldata.get('roles', ''):
        return None  # Requested roles have not yet been assigned
    if locdata.get('extensions') and locdata.get('extensions', '') != reldata.get('extensions', ''):
        return None  # Requested extensions have not yet been installed
    return ConnectionString(**d)
