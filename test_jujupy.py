__metaclass__ = type

from datetime import timedelta
from mock import patch
from textwrap import dedent
from unittest import TestCase

from jujupy import (
    Environment,
    ErroredUnit,
    JujuClient16,
    JujuClientDevel,
    until_timeout,
)


class TestErroredUnit(TestCase):

    def test_output(self):
        e = ErroredUnit('foo', 'bar', 'baz')
        self.assertEqual('<foo> bar is in state baz', str(e))


class TestUntilTimeout(TestCase):

    def test_no_timeout(self):

        iterator = until_timeout(0)

        def now_iter():
            yield iterator.start
            yield iterator.start
            assert False

        with patch.object(iterator, 'now', now_iter().next):
            for x in iterator:
                self.assertIs(None, x)
                break

    def test_timeout(self):
        iterator = until_timeout(5)

        def now_iter():
            yield iterator.start
            yield iterator.start + timedelta(0, 4)
            yield iterator.start + timedelta(0, 5)
            assert False

        with patch.object(iterator, 'now', now_iter().next):
            results = list(iterator)
        self.assertEqual([None, None], results)


class JujuClientDevelFake(JujuClientDevel):

    output_iterator = None

    @classmethod
    def get_juju_output(cls, environment, command):
        return cls.output_iterator.send((environment, command))

    @classmethod
    def set_output(cls, iterator):
        iterator.next()
        cls.output_iterator = iterator


class TestJujuClientDevel(TestCase):

    def test_get_version(self):

        def juju_cmd_iterator():
            params = yield
            self.assertEqual((None, '--version'), params)
            yield ' 5.6 \n'

        JujuClientDevelFake.set_output(juju_cmd_iterator())
        version = JujuClientDevelFake.get_version()
        self.assertEqual('5.6', version)

    def test_by_version(self):
        def juju_cmd_iterator():
            yield
            yield '1.17'
            yield '1.16'
            yield '1.16.1'
            yield '1.15'

        JujuClientDevelFake.set_output(juju_cmd_iterator())
        self.assertIs(JujuClientDevel, JujuClientDevelFake.by_version())
        self.assertIs(JujuClient16, JujuClientDevelFake.by_version())
        self.assertIs(JujuClient16, JujuClientDevelFake.by_version())
        self.assertIs(JujuClientDevel, JujuClientDevelFake.by_version())

    def test_full_args(self):
        env = Environment('foo')
        full = JujuClientDevel._full_args(env, 'bar', False, ('baz', 'qux'))
        self.assertEqual(('juju', 'bar', '-e', 'foo', 'baz', 'qux'), full)
        full = JujuClientDevel._full_args(env, 'bar', True, ('baz', 'qux'))
        self.assertEqual(('sudo', 'juju', 'bar', '-e', 'foo', 'baz', 'qux'),
                         full)
        full = JujuClientDevel._full_args(None, 'bar', False, ('baz', 'qux'))
        self.assertEqual(('juju', 'bar', 'baz', 'qux'), full)

    def test_bootstrap_non_sudo(self):
        env = Environment('foo')
        with patch.object(env, 'needs_sudo', lambda: False):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel.bootstrap(env)
            mock.assert_called_with(
                env, 'bootstrap', ('--constraints', 'mem=2G'), False)

    def test_bootstrap_sudo(self):
        env = Environment('foo')
        with patch.object(env, 'needs_sudo', lambda: True):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel.bootstrap(env)
            mock.assert_called_with(
                env, 'bootstrap', ('--constraints', 'mem=2G'), True)

    def test_destroy_environment_non_sudo(self):
        env = Environment('foo')
        with patch.object(env, 'needs_sudo', lambda: False):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel.destroy_environment(env)
            mock.assert_called_with(
                None, 'destroy-environment', ('foo', '-y'), False, check=False)

    def test_bootstrap_sudo(self):
        env = Environment('foo')
        with patch.object(env, 'needs_sudo', lambda: True):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel.destroy_environment(env)
            mock.assert_called_with(
                None, 'destroy-environment', ('foo', '-y'), True, check=False)

    def test_get_juju_output(self):
        env = Environment('foo')
        asdf = lambda x: 'asdf'
        with patch('subprocess.check_output', side_effect=asdf) as mock:
            result = JujuClientDevel.get_juju_output(env, 'bar')
        self.assertEqual('asdf', result)
        mock.assert_called_with(('juju', 'bar', '-e', 'foo'))

    def test_get_status(self):
        def output_iterator():
            args = yield
            yield dedent("""\
                - a
                - b
                - c
                """)
        JujuClientDevelFake.set_output(output_iterator())
        env = Environment('foo')
        result = JujuClientDevelFake.get_status(env)
        self.assertEqual(['a', 'b', 'c'], result)

    def test_juju(self):
        env = Environment('qux')
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.check_call') as mock:
                JujuClientDevel.juju(env, 'foo', ('bar', 'baz'))
        mock.assert_called_with(('juju', 'foo', '-e', 'qux', 'bar', 'baz'))
        stdout_mock.flush.assert_called_with()

    def test_juju_no_check(self):
        env = Environment('qux')
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.call') as mock:
                JujuClientDevel.juju(env, 'foo', ('bar', 'baz'), check=False)
        mock.assert_called_with(('juju', 'foo', '-e', 'qux', 'bar', 'baz'))
        stdout_mock.flush.assert_called_with()
