from argparse import (
    ArgumentParser,
    Namespace,
)
from datetime import (
    datetime,
    timedelta,
    )
from contextlib import contextmanager
import os
import socket
from StringIO import StringIO
from time import time
from unittest import TestCase

from mock import (
    call,
    patch,
    )

from utility import (
    add_basic_testing_arguments,
    extract_deb,
    find_candidates,
    get_auth_token,
    get_candidates_path,
    get_deb_arch,
    temp_dir,
    until_timeout,
    wait_for_port,
    )


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

    @contextmanager
    def patched_until(self, timeout, deltas):
        iterator = until_timeout(timeout)

        def now_iter():
            for d in deltas:
                yield iterator.start + d
            assert False
        with patch.object(iterator, 'now', now_iter().next):
            yield iterator

    def test_timeout(self):
        with self.patched_until(
                5, [timedelta(), timedelta(0, 4), timedelta(0, 5)]) as until:
            results = list(until)
        self.assertEqual([5, 1], results)

    def test_long_timeout(self):
        deltas = [timedelta(), timedelta(4, 0), timedelta(5, 0)]
        with self.patched_until(86400 * 5, deltas) as until:
            self.assertEqual([86400 * 5, 86400], list(until))

    def test_start(self):
        now = datetime.now() + timedelta(days=1)
        now_iter = iter([now, now, now + timedelta(10)])
        with patch('utility.until_timeout.now', side_effect=now_iter.next):
            self.assertEqual(list(until_timeout(10, now - timedelta(10))), [])


def write_config(root, job_name, token):
    job_dir = os.path.join(root, 'jobs', job_name)
    os.makedirs(job_dir)
    job_config = os.path.join(job_dir, 'config.xml')
    with open(job_config, 'w') as config:
        config.write(
            '<config><authToken>{}</authToken></config>'.format(token))


@contextmanager
def parse_error(test_case):
    stderr = StringIO()
    with test_case.assertRaises(SystemExit):
        with patch('sys.stderr', stderr):
            yield stderr


class TestGetAuthToken(TestCase):

    def test_get_auth_token(self):
        with temp_dir() as root:
            write_config(root, 'job-name', 'foo')
            self.assertEqual(get_auth_token(root, 'job-name'), 'foo')


class TestFindCandidates(TestCase):

    def test_find_candidates(self):
        with temp_dir() as root:
            candidates_path = get_candidates_path(root)
            os.mkdir(candidates_path)
            self.assertEqual(list(find_candidates(root)), [])
            master_path = os.path.join(candidates_path, 'master')
            os.mkdir(master_path)
            self.assertEqual(list(find_candidates(root)), [])
            open(os.path.join(master_path, 'buildvars.json'), 'w')
            self.assertEqual(list(find_candidates(root)), [master_path])

    def test_find_candidates_old_buildvars(self):
        with temp_dir() as root:
            candidates_path = get_candidates_path(root)
            os.mkdir(candidates_path)
            master_path = os.path.join(candidates_path, 'master')
            os.mkdir(master_path)
            buildvars_path = os.path.join(master_path, 'buildvars.json')
            open(buildvars_path, 'w')
            a_week_ago = time() - timedelta(days=7, seconds=1).total_seconds()
            os.utime(buildvars_path, (time(), a_week_ago))
            self.assertEqual(list(find_candidates(root)), [])

    def test_find_candidates_artifacts(self):
        with temp_dir() as root:
            candidates_path = get_candidates_path(root)
            os.mkdir(candidates_path)
            master_path = os.path.join(candidates_path, 'master-artifacts')
            os.mkdir(master_path)
            open(os.path.join(master_path, 'buildvars.json'), 'w')
            self.assertEqual(list(find_candidates(root)), [])


class TestWaitForPort(TestCase):

    def test_wait_for_port_0000_closed(self):
        with patch(
                'socket.getaddrinfo', autospec=True,
                return_value=[('foo', 'bar', 'baz', 'qux', ('0.0.0.0', 27))]
                ) as gai_mock:
            with patch('socket.socket', autospec=True) as socket_mock:
                wait_for_port('asdf', 26, closed=True)
        gai_mock.assert_called_once_with('asdf', 26, socket.AF_INET,
                                         socket.SOCK_STREAM)
        self.assertEqual(socket_mock.call_count, 0)

    def test_wait_for_port_0000_open(self):
        stub_called = False
        loc = locals()

        def gai_stub(host, port, family, socktype):
            if loc['stub_called']:
                raise ValueError()
            loc['stub_called'] = True
            return [('foo', 'bar', 'baz', 'qux', ('0.0.0.0', 27))]

        with patch('socket.getaddrinfo', autospec=True, side_effect=gai_stub,
                   ) as gai_mock:
            with patch('socket.socket', autospec=True) as socket_mock:
                with self.assertRaises(ValueError):
                    wait_for_port('asdf', 26, closed=False)
        self.assertEqual(gai_mock.mock_calls, [
            call('asdf', 26, socket.AF_INET, socket.SOCK_STREAM),
            call('asdf', 26, socket.AF_INET, socket.SOCK_STREAM),
            ])
        self.assertEqual(socket_mock.call_count, 0)

    def test_wait_for_port(self):
        with patch(
                'socket.getaddrinfo', autospec=True, return_value=[
                    ('foo', 'bar', 'baz', 'qux', ('192.168.8.3', 27))
                    ]) as gai_mock:
            with patch('socket.socket', autospec=True) as socket_mock:
                wait_for_port('asdf', 26, closed=False)
        gai_mock.assert_called_once_with(
            'asdf', 26, socket.AF_INET, socket.SOCK_STREAM),
        socket_mock.assert_called_once_with('foo', 'bar', 'baz')
        connect_mock = socket_mock.return_value.connect
        connect_mock.assert_called_once_with(('192.168.8.3', 27))

    def test_wait_for_port_no_address_closed(self):
        with patch('socket.getaddrinfo', autospec=True,
                   side_effect=socket.error(-5, None)) as gai_mock:
            with patch('socket.socket', autospec=True) as socket_mock:
                wait_for_port('asdf', 26, closed=True)
        gai_mock.assert_called_once_with('asdf', 26, socket.AF_INET,
                                         socket.SOCK_STREAM)
        self.assertEqual(socket_mock.call_count, 0)

    def test_wait_for_port_no_address_open(self):
        stub_called = False
        loc = locals()

        def gai_stub(host, port, family, socktype):
            if loc['stub_called']:
                raise ValueError()
            loc['stub_called'] = True
            raise socket.error(-5, None)

        with patch('socket.getaddrinfo', autospec=True, side_effect=gai_stub,
                   ) as gai_mock:
            with patch('socket.socket', autospec=True) as socket_mock:
                with self.assertRaises(ValueError):
                    wait_for_port('asdf', 26, closed=False)
        self.assertEqual(gai_mock.mock_calls, [
            call('asdf', 26, socket.AF_INET, socket.SOCK_STREAM),
            call('asdf', 26, socket.AF_INET, socket.SOCK_STREAM),
            ])
        self.assertEqual(socket_mock.call_count, 0)


class TestExtractDeb(TestCase):

    def test_extract_deb(self):
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            extract_deb('foo', 'bar')
        cc_mock.assert_called_once_with(['dpkg', '-x', 'foo', 'bar'])


class TestGetDebArch(TestCase):

    def test_get_deb_arch(self):
        with patch('subprocess.check_output',
                   return_value=' amd42 \n') as co_mock:
            arch = get_deb_arch()
        co_mock.assert_called_once_with(['dpkg', '--print-architecture'])
        self.assertEqual(arch, 'amd42')


class TestAddBasicTestingArguments(TestCase):

    def test_add_basic_testing_arguments_positional_args(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        expected = Namespace(
            agent_url=None, debug=False, env='local', job_name='testtest',
            juju_bin='/foo/juju', logs='/tmp/logs', series=None,
            verbose='logging.INFO')
        self.assertEqual(args, expected)

    def test_add_basic_testing_arguments_debug(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest', '--debug']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        expected = Namespace(
            agent_url=None, debug=True, env='local', job_name='testtest',
            juju_bin='/foo/juju', logs='/tmp/logs', series=None,
            verbose='logging.INFO')
        self.assertEqual(args, expected)

    def test_add_basic_testing_arguments_verbose(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest', '--verbose']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        expected = Namespace(
            agent_url=None, debug=False, env='local', job_name='testtest',
            juju_bin='/foo/juju', logs='/tmp/logs', series=None,
            verbose='logging.DEBUG')
        self.assertEqual(args, expected)

    def test_add_basic_testing_arguments_agent_url(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--agent-url', 'http://example.org']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        expected = Namespace(
            agent_url='http://example.org', debug=False, env='local',
            job_name='testtest', juju_bin='/foo/juju', logs='/tmp/logs',
            series=None, verbose='logging.INFO')
        self.assertEqual(args, expected)

    def test_add_basic_testing_arguments_series(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest', '--series',
                    'vivid']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        expected = Namespace(
            agent_url=None, debug=False, env='local', job_name='testtest',
            juju_bin='/foo/juju', logs='/tmp/logs', series='vivid',
            verbose='logging.INFO')
        self.assertEqual(args, expected)
