# Copyright 2014-2017 Canonical Limited.
#
# This file is part of charms.reactive
#
# charms.reactive is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License version 3 as
# published by the Free Software Foundation.
#
# charms.reactive is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with charm-helpers.  If not, see <http://www.gnu.org/licenses/>.

import os
import sys
import importlib
from inspect import isclass

from charmhelpers.core import hookenv
from charmhelpers.core import unitdata
from charmhelpers.cli import cmdline
from charms.reactive.flags import get_flags
from charms.reactive.flags import _get_flag_value
from charms.reactive.flags import set_flag
from charms.reactive.flags import clear_flag
from charms.reactive.flags import StateList
from charms.reactive.bus import _append_path

__all__ = [
    'endpoint_from_name',
    'endpoint_from_flag',
    'relation_from_flag',  # DEPRECATED
    'scopes',  # DEPRECATED
    'RelationBase',  # DEPRECATED
    'relation_from_state',  # DEPRECATED
]


# arbitrary obj instances to use as defaults instead of None
ALL = object()
TOGGLE = object()


def endpoint_from_name(endpoint_name):
    """The object used for interacting with the named relations, or None.
    """
    if endpoint_name is None:
        return None
    factory = relation_factory(endpoint_name)
    if factory:
        return factory.from_name(endpoint_name)


def relation_from_name(relation_name):
    """
    .. deprecated:: 0.6.0
       Alias for :func:`endpoint_from_name`
    """
    return endpoint_from_name(relation_name)


def endpoint_from_flag(flag):
    """The object used for interacting with relations tied to a flag, or None.
    """
    relation_name = None
    value = _get_flag_value(flag)
    if isinstance(value, dict) and 'relation' in value:
        # old-style RelationBase
        relation_name = value['relation']
    elif flag.startswith('endpoint.'):
        # new-style Endpoint
        relation_name = flag.split('.')[1]
    elif '.' in flag:
        # might be an unprefixed new-style Endpoint
        relation_name = flag.split('.')[0]
        if relation_name not in hookenv.relation_types():
            return None
    if relation_name:
        factory = relation_factory(relation_name)
        if factory:
            return factory.from_flag(flag)
    return None


def relation_from_flag(flag):
    """
    .. deprecated:: 0.6.0
       Alias for :func:`endpoint_from_flag`
    """
    return endpoint_from_flag(flag)


def relation_from_state(state):
    """
    .. deprecated:: 0.5.0
       Alias for :func:`endpoint_from_flag`
    """
    return endpoint_from_flag(state)


class RelationFactory(object):
    """Produce objects for interacting with a relation.

    Interfaces choose which RelationFactory is used for their relations
    by adding a RelationFactory subclass to
    ``$CHARM_DIR/hooks/relations/{interface}/{provides,requires,peer}.py``.
    This is normally a RelationBase subclass.
    """
    @classmethod
    def from_name(cls, relation_name):
        raise NotImplementedError()

    @classmethod
    def from_flag(cls, state):
        raise NotImplementedError()


def relation_factory(relation_name):
    """Get the RelationFactory for the given relation name.

    Looks for a RelationFactory in the first file matching:
    ``$CHARM_DIR/hooks/relations/{interface}/{provides,requires,peer}.py``
    """
    role, interface = hookenv.relation_to_role_and_interface(relation_name)
    if not (role and interface):
        hookenv.log('Unable to determine role and interface for relation '
                    '{}'.format(relation_name), hookenv.ERROR)
        return None
    return _find_relation_factory(_relation_module(role, interface))


def _relation_module(role, interface):
    """
    Return module for relation based on its role and interface, or None.

    Prefers new location (reactive/relations) over old (hooks/relations).
    """
    _append_path(hookenv.charm_dir())
    _append_path(os.path.join(hookenv.charm_dir(), 'hooks'))
    base_module = 'relations.{}.{}'.format(interface, role)
    for module in ('reactive.{}'.format(base_module), base_module):
        if module in sys.modules:
            break
        try:
            importlib.import_module(module)
            break
        except ImportError:
            continue
    else:
        hookenv.log('Unable to find implementation for relation: '
                    '{} of {}'.format(role, interface), hookenv.ERROR)
        return None
    return sys.modules[module]


def _find_relation_factory(module):
    """
    Attempt to find a RelationFactory subclass in the module.

    Note: RelationFactory and RelationBase are ignored so they may
    be imported to be used as base classes without fear.
    """
    if not module:
        return None

    # All the RelationFactory subclasses
    candidates = [o for o in (getattr(module, attr) for attr in dir(module))
                  if (o is not RelationFactory and
                      o is not RelationBase and
                      isclass(o) and
                      issubclass(o, RelationFactory))]

    # Filter out any factories that are superclasses of another factory
    # (none of the other factories subclass it). This usually makes
    # the explict check for RelationBase and RelationFactory unnecessary.
    candidates = [c1 for c1 in candidates
                  if not any(issubclass(c2, c1) for c2 in candidates
                             if c1 is not c2)]

    if not candidates:
        hookenv.log('No RelationFactory found in {}'.format(module.__name__),
                    hookenv.WARNING)
        return None

    if len(candidates) > 1:
        raise RuntimeError('Too many RelationFactory found in {}'
                           ''.format(module.__name__))

    return candidates[0]


class scopes(object):
    """
    These are the recommended scope values for relation implementations.

    To use, simply set the ``scope`` class variable to one of these::

        class MyRelationClient(RelationBase):
            scope = scopes.SERVICE
    """

    GLOBAL = 'global'
    """
    All connected services and units for this relation will share a single
    conversation.  The same data will be broadcast to every remote unit, and
    retrieved data will be aggregated across all remote units and is expected
    to either eventually agree or be set by a single leader.
    """

    SERVICE = 'service'
    """
    Each connected service for this relation will have its own conversation.
    The same data will be broadcast to every unit of each service's conversation,
    and data from all units of each service will be aggregated and is expected
    to either eventually agree or be set by a single leader.
    """

    UNIT = 'unit'
    """
    Each connected unit for this relation will have its own conversation.  This
    is the default scope.  Each unit's data will be retrieved individually, but
    note that due to how Juju works, the same data is still broadcast to all
    units of a single service.
    """


class AutoAccessors(type):
    """
    Metaclass that converts fields referenced by ``auto_accessors`` into
    accessor methods with very basic doc strings.
    """
    def __new__(cls, name, parents, dct):
        for field in dct.get('auto_accessors', []):
            meth_name = field.replace('-', '_')
            meth = cls._accessor(field)
            meth.__name__ = meth_name
            meth.__module__ = dct.get('__module__')
            meth.__doc__ = 'Get the %s, if available, or None.' % field
            dct[meth_name] = meth
        return super(AutoAccessors, cls).__new__(cls, name, parents, dct)

    @staticmethod
    def _accessor(field):
        def __accessor(self):
            return self.get_remote(field)
        return __accessor


class RelationBase(RelationFactory, metaclass=AutoAccessors):
    """
    A base class for relation implementations.
    """
    _cache = {}

    scope = scopes.UNIT
    """
    Conversation scope for this relation.

    The conversation scope controls how communication with connected units
    is aggregated into related :class:`Conversations <Conversation>`, and
    can be any of the predefined :class:`scopes`, or any arbitrary string.
    Connected units which share the same scope will be considered part of
    the same conversation.  Data sent to a conversation is sent to all units
    that are a part of that conversation, and units that are part of a
    conversation are expected to agree on the data that they send, whether
    via eventual consistency or by having a single leader set the data.

    The default scope is :attr:`scopes.UNIT`.
    """

    class states(StateList):
        """
        This is the set of :class:`States <charms.reactive.bus.State>` that this
        relation could set.

        This should be defined by the relation subclass to ensure that
        states are consistent and documented, as well as being discoverable
        and introspectable by linting and composition tools.

        For example::

            class MyRelationClient(RelationBase):
                scope = scopes.GLOBAL
                auto_accessors = ['host', 'port']

                class states(StateList):
                    connected = State('{relation_name}.connected')
                    available = State('{relation_name}.available')

                @hook('{requires:my-interface}-relation-{joined,changed}')
                def changed(self):
                    self.set_state(self.states.connected)
                    if self.host() and self.port():
                        self.set_state(self.states.available)
        """
        pass

    auto_accessors = []
    """
    Remote field names to be automatically converted into accessors with
    basic documentation.

    These accessors will just call :meth:`get_remote` using the
    :meth:`default conversation <conversation>`.  Note that it is highly
    recommended that this be used only with :attr:`scopes.GLOBAL` scope.
    """

    @classmethod
    def _startup(cls):
        # update data to be backwards compatible after fix for issue 28
        _migrate_conversations()

        if hookenv.hook_name().endswith('-relation-departed'):
            def depart_conv():
                cls(hookenv.relation_type()).conversation().depart()
            hookenv.atexit(depart_conv)

    def __init__(self, relation_name, conversations=None):
        self._relation_name = relation_name
        self._conversations = conversations or [Conversation.join(self.scope)]

    @property
    def relation_name(self):
        """
        Name of the relation this instance is handling.
        """
        return self._relation_name

    @classmethod
    def from_state(cls, state):
        """
        .. deprecated:: 0.6.1
           use :func:`endpoint_from_flag` instead
        """
        return cls.from_flag(state)

    @classmethod
    def from_flag(cls, flag):
        """
        Find relation implementation in the current charm, based on the
        name of an active flag.

        You should not use this method directly.
        Use :func:`endpoint_from_flag` instead.
        """
        value = _get_flag_value(flag)
        if value is None:
            return None
        relation_name = value['relation']
        conversations = Conversation.load(value['conversations'])
        return cls.from_name(relation_name, conversations)

    @classmethod
    def from_name(cls, relation_name, conversations=None):
        """
        Find relation implementation in the current charm, based on the
        name of the relation.

        :return: A Relation instance, or None
        """
        if relation_name is None:
            return None
        relation_class = cls._cache.get(relation_name)
        if relation_class:
            return relation_class(relation_name, conversations)
        role, interface = hookenv.relation_to_role_and_interface(relation_name)
        if role and interface:
            relation_class = cls._find_impl(role, interface)
            if relation_class:
                cls._cache[relation_name] = relation_class
                return relation_class(relation_name, conversations)
        return None

    @classmethod
    def _find_impl(cls, role, interface):
        """
        Find relation implementation based on its role and interface.
        """
        module = _relation_module(role, interface)
        if not module:
            return None
        return cls._find_subclass(module)

    @classmethod
    def _find_subclass(cls, module):
        """
        Attempt to find subclass of :class:`RelationBase` in the given module.

        Note: This means strictly subclasses and not :class:`RelationBase` itself.
        This is to prevent picking up :class:`RelationBase` being imported to be
        used as the base class.
        """
        for attr in dir(module):
            candidate = getattr(module, attr)
            if (isclass(candidate) and issubclass(candidate, cls) and
                    candidate is not RelationBase):
                return candidate
        return None

    def conversations(self):
        """
        Return a list of the conversations that this relation is currently handling.

        Note that "currently handling" means for the current state or hook context,
        and not all conversations that might be active for this relation for other
        states.
        """
        return list(self._conversations)

    def conversation(self, scope=None):
        """
        Get a single conversation, by scope, that this relation is currently handling.

        If the scope is not given, the correct scope is inferred by the current
        hook execution context.  If there is no current hook execution context, it
        is assume that there is only a single global conversation scope for this
        relation.  If this relation's scope is not global and there is no current
        hook execution context, then an error is raised.
        """
        if scope is None:
            if self.scope is scopes.UNIT:
                scope = hookenv.remote_unit()
            elif self.scope is scopes.SERVICE:
                scope = hookenv.remote_service_name()
            else:
                scope = self.scope
        if scope is None:
            raise ValueError('Unable to determine default scope: no current hook or global scope')
        for conversation in self._conversations:
            if conversation.scope == scope:
                return conversation
        else:
            raise ValueError("Conversation with scope '%s' not found" % scope)

    def set_state(self, state, scope=None):
        """
        Set the state for the :class:`Conversation` with the given scope.

        In Python, this is equivalent to::

            relation.conversation(scope).set_state(state)

        See :meth:`conversation` and :meth:`Conversation.set_state`.
        """
        self.conversation(scope).set_state(state)

    def remove_state(self, state, scope=None):
        """
        Remove the state for the :class:`Conversation` with the given scope.

        In Python, this is equivalent to::

            relation.conversation(scope).remove_state(state)

        See :meth:`conversation` and :meth:`Conversation.remove_state`.
        """
        self.conversation(scope).remove_state(state)

    def is_state(self, state, scope=None):
        """
        Test the state for the :class:`Conversation` with the given scope.

        In Python, this is equivalent to::

            relation.conversation(scope).is_state(state)

        See :meth:`conversation` and :meth:`Conversation.is_state`.
        """
        return self.conversation(scope).is_state(state)

    def toggle_state(self, state, active=TOGGLE, scope=None):
        """
        Toggle the state for the :class:`Conversation` with the given scope.

        In Python, this is equivalent to::

            relation.conversation(scope).toggle_state(state, active)

        See :meth:`conversation` and :meth:`Conversation.toggle_state`.
        """
        self.conversation(scope).toggle_state(state, active)

    def set_remote(self, key=None, value=None, data=None, scope=None, **kwdata):
        """
        Set data for the remote end(s) of the :class:`Conversation` with the given scope.

        In Python, this is equivalent to::

            relation.conversation(scope).set_remote(key, value, data, scope, **kwdata)

        See :meth:`conversation` and :meth:`Conversation.set_remote`.
        """
        self.conversation(scope).set_remote(key, value, data, **kwdata)

    def get_remote(self, key, default=None, scope=None):
        """
        Get data from the remote end(s) of the :class:`Conversation` with the given scope.

        In Python, this is equivalent to::

            relation.conversation(scope).get_remote(key, default)

        See :meth:`conversation` and :meth:`Conversation.get_remote`.
        """
        return self.conversation(scope).get_remote(key, default)

    def set_local(self, key=None, value=None, data=None, scope=None, **kwdata):
        """
        Locally store some data, namespaced by the current or given :class:`Conversation` scope.

        In Python, this is equivalent to::

            relation.conversation(scope).set_local(data, scope, **kwdata)

        See :meth:`conversation` and :meth:`Conversation.set_local`.
        """
        self.conversation(scope).set_local(key, value, data, **kwdata)

    def get_local(self, key, default=None, scope=None):
        """
        Retrieve some data previously set via :meth:`set_local`.

        In Python, this is equivalent to::

            relation.conversation(scope).get_local(key, default)

        See :meth:`conversation` and :meth:`Conversation.get_local`.
        """
        return self.conversation(scope).get_local(key, default)


class Conversation(object):
    """
    Converations are the persistent, evolving, two-way communication between
    this service and one or more remote services.

    Conversations are not limited to a single Juju hook context.  They represent
    the entire set of interactions between the end-points from the time the
    relation is joined until it is departed.

    Conversations evolve over time, moving from one semantic state to the next
    as the communication progresses.

    Conversations may encompass multiple remote services or units.  While a
    database client would connect to only a single database, that database will
    likely serve several other services.  On the other hand, while the database
    is only concerned about providing a database to each service as a whole, a
    load-balancing proxy must consider each unit of each service individually.

    Conversations use the idea of :class:`scope` to determine how units and
    services are grouped together.
    """
    def __init__(self, namespace, units, scope):
        self.namespace = namespace
        self.units = set(units)
        self.scope = scope

    @classmethod
    def _key(cls, namespace, scope):
        return 'reactive.conversations.%s.%s' % (namespace, scope)

    @property
    def key(self):
        """
        The key under which this conversation will be stored.
        """
        return self._key(self.namespace, self.scope)

    @property
    def relation_name(self):
        return self.namespace.split(':')[0]

    @property
    def relation_ids(self):
        """
        The set of IDs of the specific relation instances that this conversation
        is communicating with.
        """
        if self.scope == scopes.GLOBAL:
            # the namespace is the relation name and this conv speaks for all
            # connected instances of that relation
            return hookenv.relation_ids(self.namespace)
        else:
            # the namespace is the relation ID
            return [self.namespace]

    @classmethod
    def join(cls, scope):
        """
        Get or create a conversation for the given scope and active hook context.

        The current remote unit for the active hook context will be added to
        the conversation.

        Note: This uses :mod:`charmhelpers.core.unitdata` and requires that
        :meth:`~charmhelpers.core.unitdata.Storage.flush` be called.
        """
        relation_name = hookenv.relation_type()
        relation_id = hookenv.relation_id()
        unit = hookenv.remote_unit()
        service = hookenv.remote_service_name()
        if scope is scopes.UNIT:
            scope = unit
            namespace = relation_id
        elif scope is scopes.SERVICE:
            scope = service
            namespace = relation_id
        else:
            namespace = relation_name
        key = cls._key(namespace, scope)
        data = unitdata.kv().get(key, {'namespace': namespace, 'scope': scope, 'units': []})
        conversation = cls.deserialize(data)
        conversation.units.add(unit)
        unitdata.kv().set(key, cls.serialize(conversation))
        return conversation

    def depart(self):
        """
        Remove the current remote unit, for the active hook context, from
        this conversation.  This should be called from a `-departed` hook.
        """
        unit = hookenv.remote_unit()
        self.units.remove(unit)
        if self.units:
            unitdata.kv().set(self.key, self.serialize(self))
        else:
            unitdata.kv().unset(self.key)

    @classmethod
    def deserialize(cls, conversation):
        """
        Deserialize a :meth:`serialized <serialize>` conversation.
        """
        return cls(**conversation)

    @classmethod
    def serialize(cls, conversation):
        """
        Serialize a conversation instance for storage.
        """
        return {
            'namespace': conversation.namespace,
            'units': sorted(conversation.units),
            'scope': conversation.scope,
        }

    @classmethod
    def load(cls, keys):
        """
        Load a set of conversations by their keys.
        """
        conversations = []
        for key in keys:
            conversation = unitdata.kv().get(key)
            if conversation:
                conversations.append(cls.deserialize(conversation))
        return conversations

    def set_state(self, state):
        """
        Activate and put this conversation into the given state.

        The relation name will be interpolated in the state name, and it is
        recommended that it be included to avoid conflicts with states from
        other relations.  For example::

            conversation.set_state('{relation_name}.state')

        If called from a converation handling the relation "foo", this will
        activate the "foo.state" state, and will add this conversation to
        that state.

        Note: This uses :mod:`charmhelpers.core.unitdata` and requires that
        :meth:`~charmhelpers.core.unitdata.Storage.flush` be called.
        """
        state = state.format(relation_name=self.relation_name)
        value = _get_flag_value(state, {
            'relation': self.relation_name,
            'conversations': [],
        })
        if self.key not in value['conversations']:
            value['conversations'].append(self.key)
        set_flag(state, value)

    def remove_state(self, state):
        """
        Remove this conversation from the given state, and potentially
        deactivate the state if no more conversations are in it.

        The relation name will be interpolated in the state name, and it is
        recommended that it be included to avoid conflicts with states from
        other relations.  For example::

            conversation.remove_state('{relation_name}.state')

        If called from a converation handling the relation "foo", this will
        remove the conversation from the "foo.state" state, and, if no more
        conversations are in this the state, will deactivate it.
        """
        state = state.format(relation_name=self.relation_name)
        value = _get_flag_value(state)
        if not value:
            return
        if self.key in value['conversations']:
            value['conversations'].remove(self.key)
        if value['conversations']:
            set_flag(state, value)
        else:
            clear_flag(state)

    def is_state(self, state):
        """
        Test if this conversation is in the given state.
        """
        state = state.format(relation_name=self.relation_name)
        value = _get_flag_value(state)
        if not value:
            return False
        return self.key in value['conversations']

    def toggle_state(self, state, active=TOGGLE):
        """
        Toggle the given state for this conversation.

        The state will be set ``active`` is ``True``, otherwise the state will be removed.

        If ``active`` is not given, it will default to the inverse of the current state
        (i.e., ``False`` if the state is currently set, ``True`` if it is not; essentially
        toggling the state).

        For example::

            conv.toggle_state('{relation_name}.foo', value=='foo')

        This will set the state if ``value`` is equal to ``foo``.
        """
        if active is TOGGLE:
            active = not self.is_state(state)
        if active:
            self.set_state(state)
        else:
            self.remove_state(state)

    def set_remote(self, key=None, value=None, data=None, **kwdata):
        """
        Set data for the remote end(s) of this conversation.

        Data can be passed in either as a single dict, or as key-word args.

        Note that, in Juju, setting relation data is inherently service scoped.
        That is, if the conversation only includes a single unit, the data will
        still be set for that unit's entire service.

        However, if this conversation's scope encompasses multiple services,
        the data will be set for all of those services.

        :param str key: The name of a field to set.
        :param value: A value to set. This value must be json serializable.
        :param dict data: A mapping of keys to values.
        :param \*\*kwdata: A mapping of keys to values, as keyword arguments.
        """
        if data is None:
            data = {}
        if key is not None:
            data[key] = value
        data.update(kwdata)
        if not data:
            return
        for relation_id in self.relation_ids:
            hookenv.relation_set(relation_id, data)

    def get_remote(self, key, default=None):
        """
        Get a value from the remote end(s) of this conversation.

        Note that if a conversation's scope encompasses multiple units, then
        those units are expected to agree on their data, whether that is through
        relying on a single leader to set the data or by all units eventually
        converging to identical data.  Thus, this method returns the first
        value that it finds set by any of its units.
        """
        cur_rid = hookenv.relation_id()
        departing = hookenv.hook_name().endswith('-relation-departed')
        for relation_id in self.relation_ids:
            units = hookenv.related_units(relation_id)
            if departing and cur_rid == relation_id:
                # Work around the fact that Juju 2.0 doesn't include the
                # departing unit in relation-list during the -departed hook,
                # by adding it back in ourselves.
                units.append(hookenv.remote_unit())
            for unit in units:
                if unit not in self.units:
                    continue
                value = hookenv.relation_get(key, unit, relation_id)
                if value:
                    return value
        return default

    def set_local(self, key=None, value=None, data=None, **kwdata):
        """
        Locally store some data associated with this conversation.

        Data can be passed in either as a single dict, or as key-word args.

        For example, if you need to store the previous value of a remote field
        to determine if it has changed, you can use the following::

            prev = conversation.get_local('field')
            curr = conversation.get_remote('field')
            if prev != curr:
                handle_change(prev, curr)
                conversation.set_local('field', curr)

        Note: This uses :mod:`charmhelpers.core.unitdata` and requires that
        :meth:`~charmhelpers.core.unitdata.Storage.flush` be called.

        :param str key: The name of a field to set.
        :param value: A value to set. This value must be json serializable.
        :param dict data: A mapping of keys to values.
        :param \*\*kwdata: A mapping of keys to values, as keyword arguments.
        """
        if data is None:
            data = {}
        if key is not None:
            data[key] = value
        data.update(kwdata)
        if not data:
            return
        unitdata.kv().update(data, prefix='%s.%s.' % (self.key, 'local-data'))

    def get_local(self, key, default=None):
        """
        Retrieve some data previously set via :meth:`set_local` for this conversation.
        """
        key = '%s.%s.%s' % (self.key, 'local-data', key)
        return unitdata.kv().get(key, default)


def _migrate_conversations():  # noqa
    """
    Due to issue #28 (https://github.com/juju-solutions/charms.reactive/issues/28),
    conversations needed to be updated to be namespaced per relation ID for SERVICE
    and UNIT scope.  To ensure backwards compatibility, this updates all convs in
    the old format to the new.

    TODO: Remove in 2.0.0
    """
    for key, data in unitdata.kv().getrange('reactive.conversations.').items():
        if 'local-data' in key:
            continue
        if 'namespace' in data:
            continue
        relation_name = data.pop('relation_name')
        if data['scope'] == scopes.GLOBAL:
            data['namespace'] = relation_name
            unitdata.kv().set(key, data)
        else:
            # split the conv based on the relation ID
            new_keys = []
            for rel_id in hookenv.relation_ids(relation_name):
                new_key = Conversation._key(rel_id, data['scope'])
                new_units = set(hookenv.related_units(rel_id)) & set(data['units'])
                if new_units:
                    unitdata.kv().set(new_key, {
                        'namespace': rel_id,
                        'scope': data['scope'],
                        'units': sorted(new_units),
                    })
                    new_keys.append(new_key)
            unitdata.kv().unset(key)
            # update the states pointing to the old conv key to point to the
            # (potentially multiple) new key(s)
            for flag in get_flags():
                value = _get_flag_value(flag)
                if not value:
                    continue
                if key not in value['conversations']:
                    continue
                value['conversations'].remove(key)
                value['conversations'].extend(new_keys)
                set_flag(flag, value)


@cmdline.subcommand()
def relation_call(method, relation_name=None, flag=None, state=None, *args):
    """Invoke a method on the class implementing a relation via the CLI"""
    if relation_name:
        relation = relation_from_name(relation_name)
        if relation is None:
            raise ValueError('Relation not found: %s' % relation_name)
    elif flag or state:
        relation = relation_from_flag(flag or state)
        if relation is None:
            raise ValueError('Relation not found: %s' % (flag or state))
    else:
        raise ValueError('Must specify either relation_name or flag')
    result = getattr(relation, method)(*args)
    if isinstance(relation, RelationBase) and method == 'conversations':
        # special case for conversations to make them work from CLI
        result = [c.scope for c in result]
    return result


hookenv.atstart(RelationBase._startup)
