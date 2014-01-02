__metaclass__ = type

from datetime import timedelta
import os
import shutil
from StringIO import StringIO
import subprocess
import tempfile
from textwrap import dedent
from unittest import TestCase

from mock import (
    MagicMock,
    patch,
)
import yaml

from jujupy import (
    check_wordpress,
    Environment,
    ErroredUnit,
    format_listing,
    JujuClient16,
    JujuClientDevel,
    Status,
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
        with patch(
                'subprocess.check_output', side_effect=lambda x: ' 5.6 \n'
                ) as vsn:
            version = JujuClientDevelFake.get_version()
        self.assertEqual('5.6', version)
        vsn.assert_called_with(('juju', '--version'))


    def test_by_version(self):
        def juju_cmd_iterator():
            yield '1.17'
            yield '1.16'
            yield '1.16.1'
            yield '1.15'

        context = patch.object(JujuClientDevel, 'get_version',
                side_effect=juju_cmd_iterator().next)
        with context:
            self.assertIs(JujuClientDevel,
                          type(JujuClientDevel.by_version()))
            self.assertIs(JujuClient16, type(JujuClientDevel.by_version()))
            self.assertIs(JujuClient16, type(JujuClientDevel.by_version()))
            client = JujuClientDevel.by_version()
            self.assertIs(JujuClientDevel, type(client))
            self.assertEqual('1.15', client.version)

    def test_full_args(self):
        env = Environment('foo', '')
        client = JujuClientDevel(None, 'my/juju/bin')
        full = client._full_args(env, 'bar', False, ('baz', 'qux'))
        self.assertEqual(('juju', '--show-log', 'bar', '-e', 'foo', 'baz',
                          'qux'), full)
        full = client._full_args(env, 'bar', True, ('baz', 'qux'))
        self.assertEqual((
            'sudo', '-E', 'my/juju/bin', '--show-log', 'bar', '-e', 'foo',
            'baz', 'qux'), full)
        full = client._full_args(None, 'bar', False, ('baz', 'qux'))
        self.assertEqual(('juju', '--show-log', 'bar', 'baz', 'qux'), full)

    def test_bootstrap_hpcloud(self):
        env = Environment('hp', '')
        with patch.object(env, 'hpcloud', lambda: True):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel(None, None).bootstrap(env)
            mock.assert_called_with(
                env, 'bootstrap', ('--constraints', 'mem=4G'), False)

    def test_bootstrap_non_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: False):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel(None, None).bootstrap(env)
            mock.assert_called_with(
                env, 'bootstrap', ('--constraints', 'mem=2G'), False)

    def test_bootstrap_sudo(self):
        env = Environment('foo', '')
        client = JujuClientDevel(None, None)
        with patch.object(env, 'needs_sudo', lambda: True):
            with patch.object(JujuClientDevel, 'juju') as mock:
                client.bootstrap(env)
            mock.assert_called_with(
                env, 'bootstrap', ('--constraints', 'mem=2G'), True)

    def test_destroy_environment_non_sudo(self):
        env = Environment('foo', '')
        client = JujuClientDevel(None, None)
        with patch.object(env, 'needs_sudo', lambda: False):
            with patch.object(JujuClientDevel, 'juju') as mock:
                client.destroy_environment(env)
            mock.assert_called_with(
                None, 'destroy-environment', ('foo', '-y'), False, check=False)

    def test_destroy_environment_sudo(self):
        env = Environment('foo', '')
        client = JujuClientDevel(None, None)
        with patch.object(env, 'needs_sudo', lambda: True):
            with patch.object(JujuClientDevel, 'juju') as mock:
                client.destroy_environment(env)
            mock.assert_called_with(
                None, 'destroy-environment', ('foo', '-y'), True, check=False)

    def test_get_juju_output(self):
        env = Environment('foo', '')
        asdf = lambda x, stderr: 'asdf'
        client = JujuClientDevel(None, None)
        with patch('subprocess.check_output', side_effect=asdf) as mock:
            result = client.get_juju_output(env, 'bar')
        self.assertEqual('asdf', result)
        self.assertEqual((('juju', '--show-log', 'bar', '-e', 'foo'),),
                         mock.call_args[0])

    def test_get_juju_output_stderr(self):
        def raise_without_stderr(args, stderr):
            stderr.write('Hello!')
            raise subprocess.CalledProcessError('a', 'b')
        env = Environment('foo', '')
        client = JujuClientDevel(None, None)
        with self.assertRaises(subprocess.CalledProcessError) as exc:
            with patch('subprocess.check_output', raise_without_stderr):
                result = client.get_juju_output(env, 'bar')
        self.assertEqual(exc.exception.stderr, 'Hello!')

    def test_get_status(self):
        def output_iterator():
            args = yield
            yield dedent("""\
                - a
                - b
                - c
                """)
        client = JujuClientDevelFake(None, None)
        client.set_output(output_iterator())
        env = Environment('foo', '')
        result = client.get_status(env)
        self.assertEqual(Status, type(result))
        self.assertEqual(['a', 'b', 'c'], result.status)

    def test_juju(self):
        env = Environment('qux', '')
        client = JujuClientDevel(None, None)
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.check_call') as mock:
                client.juju(env, 'foo', ('bar', 'baz'))
        mock.assert_called_with(('juju', '--show-log', 'foo', '-e', 'qux',
                                 'bar', 'baz'))
        stdout_mock.flush.assert_called_with()

    def test_juju_no_check(self):
        env = Environment('qux', '')
        client = JujuClientDevel(None, None)
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.call') as mock:
                client.juju(env, 'foo', ('bar', 'baz'), check=False)
        mock.assert_called_with(('juju', '--show-log', 'foo', '-e', 'qux',
                                 'bar', 'baz'))
        stdout_mock.flush.assert_called_with()


class TestJujuClient16(TestCase):

    def test_destroy_environment_non_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: False):
            with patch.object(JujuClient16, 'juju') as mock:
                JujuClient16(None, None).destroy_environment(env)
            mock.assert_called_with(
                env, 'destroy-environment', ('-y',), False, check=False)

    def test_destroy_environment_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: True):
            with patch.object(JujuClient16, 'juju') as mock:
                JujuClient16(None, None).destroy_environment(env)
            mock.assert_called_with(
                env, 'destroy-environment', ('-y',), True, check=False)


class TestStatus(TestCase):

    def test_agent_items_empty(self):
        status = Status({'machines': {}, 'services': {}})
        self.assertItemsEqual([], status.agent_items())

    def test_agent_items(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar'}
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'baz': 'qux'}
                    }
                }
            }
        })
        expected = [
            ('1', {'foo': 'bar'}), ('jenkins/1', {'baz': 'qux'})]
        self.assertItemsEqual(expected, status.agent_items())

    def test_agent_states(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'good'},
                    }
                }
            }
        })
        expected = {
            'good': ['1', 'jenkins/2'],
            'bad': ['jenkins/1'],
            'no-agent': ['2'],
        }
        self.assertEqual(expected, status.agent_states())

    def test_check_agents_started_not_started(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'good'},
                    }
                }
            }
        })
        self.assertEqual(status.agent_states(),
                         status.check_agents_started('env1'))

    def test_check_agents_started_all_started(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'started'},
                '2': {'agent-state': 'started'},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'started'},
                        'jenkins/2': {'agent-state': 'started'},
                    }
                }
            }
        })
        self.assertIs(None, status.check_agents_started('env1'))

    def test_check_agents_started_agent_error(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'any-error'},
            },
            'services': {}
        })
        with self.assertRaisesRegexp(ErroredUnit,
                                     '<env1> 1 is in state any-error'):
            status.check_agents_started('env1')

    def test_check_agents_started_agent_info_error(self):
        # Sometimes the error is indicated in a special 'agent-state-info'
        # field.
        status = Status({
            'machines': {
                '1': {'agent-state-info': 'any-error'},
            },
            'services': {}
        })
        with self.assertRaisesRegexp(ErroredUnit,
                                     '<env1> 1 is in state any-error'):
            status.check_agents_started('env1')

    def test_get_agent_versions(self):
        status = Status({
            'machines': {
                '1': {'agent-version': '1.6.2'},
                '2': {'agent-version': '1.6.1'},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/0':
                            {'agent-version': '1.6.1'},
                        'jenkins/1': {},
                    },
                }
            }
        })
        self.assertEqual({
            '1.6.2': {'1'},
            '1.6.1': {'jenkins/0', '2'},
            'unknown': {'jenkins/1'},
        }, status.get_agent_versions())


def fast_timeout(count):
    if False:
        yield


class TestEnvironment(TestCase):

    def test_wait_for_started(self):
        def output_iterator():
            yield
            yield dedent("""\
                machines:
                  "0":
                    agent-state: started
                services:
                  jenkins:
                    units:
                      jenkins/0:
                        agent-state: started
            """)
        JujuClientDevelFake.set_output(output_iterator())
        env = Environment('local', JujuClientDevelFake(None, None))
        env.wait_for_started()

    def test_wait_for_started_timeout(self):
        def output_iterator():
            yield
            while True:
                yield dedent("""\
                    machines:
                      "0":
                        agent-state: pending
                    services:
                      jenkins:
                        units:
                          jenkins/0:
                            agent-state: started
                """)
        JujuClientDevelFake.set_output(output_iterator())
        env = Environment('local', JujuClientDevelFake)
        with patch('jujupy.until_timeout', lambda x: range(0)):
            with self.assertRaisesRegexp(
                    Exception,
                    'Timed out waiting for agents to start in local'):
                env.wait_for_started()

    def test_wait_for_version(self):
        def output_iterator():
            yield
            yield dedent("""\
                machines:
                  "0":
                    agent-version: 1.17.2
                services:
                  jenkins:
                    units:
                      jenkins/0:
                        agent-version: 1.17.2
            """)
        JujuClientDevelFake.set_output(output_iterator())
        env = Environment('local', JujuClientDevelFake(None, None))
        env.wait_for_version('1.17.2')

    def test_wait_for_version_timeout(self):
        def output_iterator():
            yield
            yield dedent("""\
                machines:
                  "0":
                    agent-version: 1.17.2
                services:
                  jenkins:
                    units:
                      jenkins/0:
                        agent-version: 1.17.1
            """)
        JujuClientDevelFake.set_output(output_iterator())
        env = Environment('local', JujuClientDevelFake)
        with patch('jujupy.until_timeout', lambda x: range(0)):
            with self.assertRaisesRegexp(
                    Exception, 'Some versions did not update'):
                env.wait_for_version('1.17.2')

    def test_local_from_config(self):
        env = Environment('local', '', {'type': 'openstack'})
        self.assertFalse(env.local, 'Does not respect config type.')
        env = Environment('local', '', {'type': 'local'})
        self.assertTrue(env.local, 'Does not respect config type.')

    def test_hpcloud_from_config(self):
        env = Environment('cloud', '', {'auth-url': 'before.keystone.after'})
        self.assertFalse(env.hpcloud, 'Does not respect config type.')
        env = Environment('hp', '', {'auth-url': 'before.hpcloudsvc.after/'})
        self.assertTrue(env.hpcloud, 'Does not respect config type.')

    def test_from_config(self):
        home = tempfile.mkdtemp()
        try:
            environments_path = os.path.join(home, 'environments.yaml')
            old_home = os.environ.get('JUJU_HOME')
            os.environ['JUJU_HOME'] = home
            try:
                with open(environments_path, 'w') as environments:
                    yaml.dump({'environments': {
                        'foo': {'type': 'local'}
                    }}, environments)
                env = Environment.from_config('foo')
                self.assertIs(Environment, type(env))
                self.assertEqual({'type': 'local'}, env.config)
            finally:
                if old_home is None:
                    del os.environ['JUJU_HOME']
                else:
                    os.environ['JUJU_HOME'] = old_home
        finally:
            shutil.rmtree(home)

    def test_upgrade_juju_nonlocal(self):
        env = Environment('foo', MagicMock(), {'type': 'nonlocal'})
        env.client.version = '1.234-76'
        env.upgrade_juju()
        env.client.juju.assert_called_with(env, 'upgrade-juju',
                                           ('--version', '1.234'))

    def test_get_matching_agent_version(self):
        env = Environment('foo', MagicMock(), {'type': 'local'})
        env.client.version = '1.234-76'
        self.assertEqual('1.234.1', env.get_matching_agent_version())
        self.assertEqual('1.234', env.get_matching_agent_version(
                         no_build=True))

    def test_upgrade_juju_local(self):
        env = Environment('foo', MagicMock(), {'type': 'local'})
        env.client.version = '1.234-76'
        env.upgrade_juju()
        env.client.juju.assert_called_with(
            env, 'upgrade-juju', ('--version', '1.234', '--upload-tools',))


class TestFormatListing(TestCase):

    def test_format_listing(self):
        result = format_listing(
            {'1': ['a', 'b'], '2': ['c'], 'expected': ['d']}, 'expected', 'e')
        self.assertEqual('<e> 1: a, b | 2: c', result)


class TestCheckWordpress(TestCase):

    def test_check_wordpress(self):
        out = StringIO('Welcome to the famous five minute WordPress'
                       ' installation process!')
        with patch('urllib2.urlopen', side_effect=lambda x: out) as mock:
            check_wordpress('foo', 'host')
        mock.assert_called_with('http://host/wp-admin/install.php')

    def test_check_wordpress_failure(self):
        out = StringIO('Urk!')
        sleep = patch('jujupy.sleep')
        urlopen = patch('urllib2.urlopen', side_effect=lambda x: out)
        timeout = patch('jujupy.until_timeout', lambda x: range(1))
        with sleep, urlopen, timeout:
            with self.assertRaisesRegexp(
                    Exception, 'Cannot get welcome screen at .*host.* foo'):
                check_wordpress('foo', 'host')
