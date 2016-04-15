#!/usr/bin/env python
# -*- coding: utf-8 -*-
#
# Copyright 2014-2015 Canonical Limited.
#
# This file is part of charm-helpers.
#
# charm-helpers is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License version 3 as
# published by the Free Software Foundation.
#
# charm-helpers is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with charm-helpers.  If not, see <http://www.gnu.org/licenses/>.
#
#
# Authors:
#  Kapil Thangavelu <kapil.foss@gmail.com>
#
"""
Intro
-----

A simple way to store state in units. This provides a key value
storage with support for versioned, transactional operation,
and can calculate deltas from previous values to simplify unit logic
when processing changes.


Hook Integration
----------------

There are several extant frameworks for hook execution, including

 - charmhelpers.core.hookenv.Hooks
 - charmhelpers.core.services.ServiceManager

The storage classes are framework agnostic, one simple integration is
via the HookData contextmanager. It will record the current hook
execution environment (including relation data, config data, etc.),
setup a transaction and allow easy access to the changes from
previously seen values. One consequence of the integration is the
reservation of particular keys ('rels', 'unit', 'env', 'config',
'charm_revisions') for their respective values.

Here's a fully worked integration example using hookenv.Hooks::

       from charmhelper.core import hookenv, unitdata

       hook_data = unitdata.HookData()
       db = unitdata.kv()
       hooks = hookenv.Hooks()

       @hooks.hook
       def config_changed():
           # Print all changes to configuration from previously seen
           # values.
           for changed, (prev, cur) in hook_data.conf.items():
               print('config changed', changed,
                     'previous value', prev,
                     'current value',  cur)

           # Get some unit specific bookeeping
           if not db.get('pkg_key'):
               key = urllib.urlopen('https://example.com/pkg_key').read()
               db.set('pkg_key', key)

           # Directly access all charm config as a mapping.
           conf = db.getrange('config', True)

           # Directly access all relation data as a mapping
           rels = db.getrange('rels', True)

       if __name__ == '__main__':
           with hook_data():
               hook.execute()


A more basic integration is via the hook_scope context manager which simply
manages transaction scope (and records hook name, and timestamp)::

  >>> from unitdata import kv
  >>> db = kv()
  >>> with db.hook_scope('install'):
  ...    # do work, in transactional scope.
  ...    db.set('x', 1)
  >>> db.get('x')
  1


Usage
-----

Values are automatically json de/serialized to preserve basic typing
and complex data struct capabilities (dicts, lists, ints, booleans, etc).

Individual values can be manipulated via get/set::

   >>> kv.set('y', True)
   >>> kv.get('y')
   True

   # We can set complex values (dicts, lists) as a single key.
   >>> kv.set('config', {'a': 1, 'b': True'})

   # Also supports returning dictionaries as a record which
   # provides attribute access.
   >>> config = kv.get('config', record=True)
   >>> config.b
   True


Groups of keys can be manipulated with update/getrange::

   >>> kv.update({'z': 1, 'y': 2}, prefix="gui.")
   >>> kv.getrange('gui.', strip=True)
   {'z': 1, 'y': 2}

When updating values, its very helpful to understand which values
have actually changed and how have they changed. The storage
provides a delta method to provide for this::

   >>> data = {'debug': True, 'option': 2}
   >>> delta = kv.delta(data, 'config.')
   >>> delta.debug.previous
   None
   >>> delta.debug.current
   True
   >>> delta
   {'debug': (None, True), 'option': (None, 2)}

Note the delta method does not persist the actual change, it needs to
be explicitly saved via 'update' method::

   >>> kv.update(data, 'config.')

Values modified in the context of a hook scope retain historical values
associated to the hookname.

   >>> with db.hook_scope('config-changed'):
   ...      db.set('x', 42)
   >>> db.gethistory('x')
   [(1, u'x', 1, u'install', u'2015-01-21T16:49:30.038372'),
    (2, u'x', 42, u'config-changed', u'2015-01-21T16:49:30.038786')]

"""

import collections
import contextlib
import datetime
import json
import os
import pprint
import sqlite3
import sys

__author__ = 'Kapil Thangavelu <kapil.foss@gmail.com>'


class Storage(object):
    """Simple key value database for local unit state within charms.

    Modifications are automatically committed at hook exit. That's
    currently regardless of exit code.

    To support dicts, lists, integer, floats, and booleans values
    are automatically json encoded/decoded.
    """
    def __init__(self, path=None):
        self.db_path = path
        if path is None:
            self.db_path = os.path.join(
                os.environ.get('CHARM_DIR', ''), '.unit-state.db')
        self.conn = sqlite3.connect('%s' % self.db_path)
        self.cursor = self.conn.cursor()
        self.revision = None
        self._closed = False
        self._init()

    def close(self):
        if self._closed:
            return
        self.flush(False)
        self.cursor.close()
        self.conn.close()
        self._closed = True

    def _scoped_query(self, stmt, params=None):
        if params is None:
            params = []
        return stmt, params

    def get(self, key, default=None, record=False):
        self.cursor.execute(
            *self._scoped_query(
                'select data from kv where key=?', [key]))
        result = self.cursor.fetchone()
        if not result:
            return default
        if record:
            return Record(json.loads(result[0]))
        return json.loads(result[0])

    def getrange(self, key_prefix, strip=False):
        stmt = "select key, data from kv where key like '%s%%'" % key_prefix
        self.cursor.execute(*self._scoped_query(stmt))
        result = self.cursor.fetchall()

        if not result:
            return None
        if not strip:
            key_prefix = ''
        return dict([
            (k[len(key_prefix):], json.loads(v)) for k, v in result])

    def update(self, mapping, prefix=""):
        for k, v in mapping.items():
            self.set("%s%s" % (prefix, k), v)

    def unset(self, key):
        self.cursor.execute('delete from kv where key=?', [key])
        if self.revision and self.cursor.rowcount:
            self.cursor.execute(
                'insert into kv_revisions values (?, ?, ?)',
                [key, self.revision, json.dumps('DELETED')])

    def set(self, key, value):
        serialized = json.dumps(value)

        self.cursor.execute(
            'select data from kv where key=?', [key])
        exists = self.cursor.fetchone()

        # Skip mutations to the same value
        if exists:
            if exists[0] == serialized:
                return value

        if not exists:
            self.cursor.execute(
                'insert into kv (key, data) values (?, ?)',
                (key, serialized))
        else:
            self.cursor.execute('''
            update kv
            set data = ?
            where key = ?''', [serialized, key])

        # Save
        if not self.revision:
            return value

        self.cursor.execute(
            'select 1 from kv_revisions where key=? and revision=?',
            [key, self.revision])
        exists = self.cursor.fetchone()

        if not exists:
            self.cursor.execute(
                '''insert into kv_revisions (
                revision, key, data) values (?, ?, ?)''',
                (self.revision, key, serialized))
        else:
            self.cursor.execute(
                '''
                update kv_revisions
                set data = ?
                where key = ?
                and   revision = ?''',
                [serialized, key, self.revision])

        return value

    def delta(self, mapping, prefix):
        """
        return a delta containing values that have changed.
        """
        previous = self.getrange(prefix, strip=True)
        if not previous:
            pk = set()
        else:
            pk = set(previous.keys())
        ck = set(mapping.keys())
        delta = DeltaSet()

        # added
        for k in ck.difference(pk):
            delta[k] = Delta(None, mapping[k])

        # removed
        for k in pk.difference(ck):
            delta[k] = Delta(previous[k], None)

        # changed
        for k in pk.intersection(ck):
            c = mapping[k]
            p = previous[k]
            if c != p:
                delta[k] = Delta(p, c)

        return delta

    @contextlib.contextmanager
    def hook_scope(self, name=""):
        """Scope all future interactions to the current hook execution
        revision."""
        assert not self.revision
        self.cursor.execute(
            'insert into hooks (hook, date) values (?, ?)',
            (name or sys.argv[0],
             datetime.datetime.utcnow().isoformat()))
        self.revision = self.cursor.lastrowid
        try:
            yield self.revision
            self.revision = None
        except:
            self.flush(False)
            self.revision = None
            raise
        else:
            self.flush()

    def flush(self, save=True):
        if save:
            self.conn.commit()
        elif self._closed:
            return
        else:
            self.conn.rollback()

    def _init(self):
        self.cursor.execute('''
            create table if not exists kv (
               key text,
               data text,
               primary key (key)
               )''')
        self.cursor.execute('''
            create table if not exists kv_revisions (
               key text,
               revision integer,
               data text,
               primary key (key, revision)
               )''')
        self.cursor.execute('''
            create table if not exists hooks (
               version integer primary key autoincrement,
               hook text,
               date text
               )''')
        self.conn.commit()

    def gethistory(self, key, deserialize=False):
        self.cursor.execute(
            '''
            select kv.revision, kv.key, kv.data, h.hook, h.date
            from kv_revisions kv,
                 hooks h
            where kv.key=?
             and kv.revision = h.version
            ''', [key])
        if deserialize is False:
            return self.cursor.fetchall()
        return map(_parse_history, self.cursor.fetchall())

    def debug(self, fh=sys.stderr):
        self.cursor.execute('select * from kv')
        pprint.pprint(self.cursor.fetchall(), stream=fh)
        self.cursor.execute('select * from kv_revisions')
        pprint.pprint(self.cursor.fetchall(), stream=fh)


def _parse_history(d):
    return (d[0], d[1], json.loads(d[2]), d[3],
            datetime.datetime.strptime(d[-1], "%Y-%m-%dT%H:%M:%S.%f"))


class HookData(object):
    """Simple integration for existing hook exec frameworks.

    Records all unit information, and stores deltas for processing
    by the hook.

    Sample::

       from charmhelper.core import hookenv, unitdata

       changes = unitdata.HookData()
       db = unitdata.kv()
       hooks = hookenv.Hooks()

       @hooks.hook
       def config_changed():
           # View all changes to configuration
           for changed, (prev, cur) in changes.conf.items():
               print('config changed', changed,
                     'previous value', prev,
                     'current value',  cur)

           # Get some unit specific bookeeping
           if not db.get('pkg_key'):
               key = urllib.urlopen('https://example.com/pkg_key').read()
               db.set('pkg_key', key)

       if __name__ == '__main__':
           with changes():
               hook.execute()

    """
    def __init__(self):
        self.kv = kv()
        self.conf = None
        self.rels = None

    @contextlib.contextmanager
    def __call__(self):
        from charmhelpers.core import hookenv
        hook_name = hookenv.hook_name()

        with self.kv.hook_scope(hook_name):
            self._record_charm_version(hookenv.charm_dir())
            delta_config, delta_relation = self._record_hook(hookenv)
            yield self.kv, delta_config, delta_relation

    def _record_charm_version(self, charm_dir):
        # Record revisions.. charm revisions are meaningless
        # to charm authors as they don't control the revision.
        # so logic dependnent on revision is not particularly
        # useful, however it is useful for debugging analysis.
        charm_rev = open(
            os.path.join(charm_dir, 'revision')).read().strip()
        charm_rev = charm_rev or '0'
        revs = self.kv.get('charm_revisions', [])
        if charm_rev not in revs:
            revs.append(charm_rev.strip() or '0')
            self.kv.set('charm_revisions', revs)

    def _record_hook(self, hookenv):
        data = hookenv.execution_environment()
        self.conf = conf_delta = self.kv.delta(data['conf'], 'config')
        self.rels = rels_delta = self.kv.delta(data['rels'], 'rels')
        self.kv.set('env', dict(data['env']))
        self.kv.set('unit', data['unit'])
        self.kv.set('relid', data.get('relid'))
        return conf_delta, rels_delta


class Record(dict):

    __slots__ = ()

    def __getattr__(self, k):
        if k in self:
            return self[k]
        raise AttributeError(k)


class DeltaSet(Record):

    __slots__ = ()


Delta = collections.namedtuple('Delta', ['previous', 'current'])


_KV = None


def kv():
    global _KV
    if _KV is None:
        _KV = Storage()
    return _KV
