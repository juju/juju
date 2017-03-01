from argparse import (
    ArgumentParser,
    Namespace,
    )
from datetime import (
    datetime,
    timedelta,
    )
from contextlib import contextmanager
import errno
import json
import logging
import os
import socket
from tempfile import mkdtemp
from time import time
import warnings

from mock import (
    call,
    Mock,
    patch,
    )

from tests import (
    TestCase,
    )
from utility import (
    add_basic_testing_arguments,
    as_literal_address,
    extract_deb,
    _find_candidates,
    find_candidates,
    find_latest_branch_candidates,
    get_candidates_path,
    get_deb_arch,
    get_winrm_certs,
    is_ipv6_address,
    log_and_wrap_exception,
    logged_exception,
    LoggedException,
    quote,
    run_command,
    scoped_environ,
    skip_on_missing_file,
    split_address_port,
    temp_dir,
    until_timeout,
    unqualified_model_name,
    qualified_model_name,
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


class TestFindCandidates(TestCase):

    def test__find_candidates_artifacts_default(self):
        with temp_dir() as root:
            make_candidate_dir(root, 'master-artifacts')
            make_candidate_dir(root, '1.25')
            candidate = os.path.join(root, 'candidate', '1.25')
            self.assertEqual(list(_find_candidates(root)), [
                (candidate, os.path.join(candidate, 'buildvars.json'))])

    def test__find_candidates_artifacts_enabled(self):
        with temp_dir() as root:
            make_candidate_dir(root, 'master-artifacts')
            make_candidate_dir(root, '1.25')
            candidate = os.path.join(root, 'candidate', 'master-artifacts')
            self.assertEqual(list(_find_candidates(root, artifacts=True)), [
                (candidate, os.path.join(candidate, 'buildvars.json'))])

    def test_find_candidates(self):
        with temp_dir() as root:
            master_path = make_candidate_dir(root, 'master')
            self.assertEqual(list(find_candidates(root)), [master_path])

    def test_find_candidates_old_buildvars(self):
        with temp_dir() as root:
            a_week_ago = time() - timedelta(days=7, seconds=1).total_seconds()
            make_candidate_dir(root, 'master', modified=a_week_ago)
            self.assertEqual(list(find_candidates(root)), [])

    def test_find_candidates_artifacts(self):
        with temp_dir() as root:
            make_candidate_dir(root, 'master-artifacts')
            self.assertEqual(list(find_candidates(root)), [])

    def test_find_candidates_find_all(self):
        with temp_dir() as root:
            a_week_ago = time() - timedelta(days=7, seconds=1).total_seconds()
            master_path = make_candidate_dir(root, '1.23', modified=a_week_ago)
            master_path_2 = make_candidate_dir(root, '1.24')
            self.assertItemsEqual(list(find_candidates(root)), [master_path_2])
            self.assertItemsEqual(list(find_candidates(root, find_all=True)),
                                  [master_path, master_path_2])


def make_candidate_dir(root, candidate_id, branch='foo', revision_build=1234,
                       modified=None):
    candidates_path = get_candidates_path(root)
    if not os.path.isdir(candidates_path):
        os.mkdir(candidates_path)
    master_path = os.path.join(candidates_path, candidate_id)
    os.mkdir(master_path)
    buildvars_path = os.path.join(master_path, 'buildvars.json')
    with open(buildvars_path, 'w') as buildvars_file:
        json.dump(
            {'branch': branch, 'revision_build': str(revision_build)},
            buildvars_file)
    if modified is not None:
        os.utime(buildvars_path, (time(), modified))
    juju_path = os.path.join(master_path, 'usr', 'foo', 'juju')
    os.makedirs(os.path.dirname(juju_path))
    with open(juju_path, 'w') as juju_file:
        juju_file.write('Fake juju bin.\n')
    return master_path


class TestFindLatestBranchCandidates(TestCase):

    def test_find_latest_branch_candidates(self):
        with temp_dir() as root:
            master_path = make_candidate_dir(root, 'master-artifacts')
            self.assertEqual(find_latest_branch_candidates(root),
                             [(master_path, 1234)])

    def test_find_latest_branch_candidates_old_buildvars(self):
        with temp_dir() as root:
            a_week_ago = time() - timedelta(days=7, seconds=1).total_seconds()
            make_candidate_dir(root, 'master-artifacts', modified=a_week_ago)
            self.assertEqual(find_latest_branch_candidates(root), [])

    def test_ignore_older_revision_build(self):
        with temp_dir() as root:
            path_1234 = make_candidate_dir(
                root, '1234-artifacts', 'mybranch', '1234')
            make_candidate_dir(root, '1233', 'mybranch', '1233')
            self.assertEqual(find_latest_branch_candidates(root), [
                (path_1234, 1234)])

    def test_include_older_revision_build_different_branch(self):
        with temp_dir() as root:
            path_1234 = make_candidate_dir(
                root, '1234-artifacts', 'branch_foo', '1234')
            path_1233 = make_candidate_dir(
                root, '1233-artifacts', 'branch_bar', '1233')
            self.assertItemsEqual(
                find_latest_branch_candidates(root), [
                    (path_1233, 1233), (path_1234, 1234)])


class TestAsLiteralAddress(TestCase):

    def test_hostname(self):
        self.assertEqual("name.testing", as_literal_address("name.testing"))

    def test_ipv4(self):
        self.assertEqual("127.0.0.2", as_literal_address("127.0.0.2"))

    def test_ipv6(self):
        self.assertEqual("[2001:db8::7]", as_literal_address("2001:db8::7"))


class TestIsIPv6Address(TestCase):

    def test_hostname(self):
        self.assertIs(False, is_ipv6_address("name.testing"))

    def test_ipv4(self):
        self.assertIs(False, is_ipv6_address("127.0.0.2"))

    def test_ipv6(self):
        self.assertIs(True, is_ipv6_address("2001:db8::4"))

    def test_ipv6_missing_support(self):
        with patch('utility.socket', wraps=socket) as wrapped_socket:
            del wrapped_socket.inet_pton
            result = is_ipv6_address("2001:db8::4")
        # Would use expectedFailure here, but instead just assert wrong result.
        self.assertIs(False, result)


class TestSplitAddressPort(TestCase):

    def test_hostname(self):
        self.assertEqual(
            ("name.testing", None), split_address_port("name.testing"))

    def test_ipv4(self):
        self.assertEqual(
            ("127.0.0.2", "17017"), split_address_port("127.0.0.2:17017"))

    def test_ipv6(self):
        self.assertEqual(
            ("2001:db8::7", "17017"), split_address_port("2001:db8::7:17017"))


class TestWaitForPort(TestCase):

    def test_wait_for_port_0000_closed(self):
        with patch(
                'socket.getaddrinfo', autospec=True,
                return_value=[('foo', 'bar', 'baz', 'qux', ('0.0.0.0', 27))]
                ) as gai_mock:
            with patch('socket.socket') as socket_mock:
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
            with patch('socket.socket') as socket_mock:
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
            with patch('socket.socket') as socket_mock:
                wait_for_port('asdf', 26, closed=False)
        gai_mock.assert_called_once_with(
            'asdf', 26, socket.AF_INET, socket.SOCK_STREAM),
        socket_mock.assert_called_once_with('foo', 'bar', 'baz')
        connect_mock = socket_mock.return_value.connect
        connect_mock.assert_called_once_with(('192.168.8.3', 27))

    def test_wait_for_port_no_address_closed(self):
        error = socket.gaierror(socket.EAI_NODATA, 'What address?')
        with patch('socket.getaddrinfo', autospec=True,
                   side_effect=error) as gai_mock:
            with patch('socket.socket') as socket_mock:
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
            raise socket.error(socket.EAI_NODATA, 'Err, address?')

        with patch('socket.getaddrinfo', autospec=True, side_effect=gai_stub,
                   ) as gai_mock:
            with patch('socket.socket') as socket_mock:
                with self.assertRaises(ValueError):
                    wait_for_port('asdf', 26, closed=False)
        self.assertEqual(gai_mock.mock_calls, [
            call('asdf', 26, socket.AF_INET, socket.SOCK_STREAM),
            call('asdf', 26, socket.AF_INET, socket.SOCK_STREAM),
            ])
        self.assertEqual(socket_mock.call_count, 0)

    def test_ipv6_open(self):
        gai_result = [(23, 0, 0, '', ('2001:db8::2', 22, 0, 0))]
        with patch('socket.getaddrinfo', autospec=True,
                   return_value=gai_result) as gai_mock:
            with patch('socket.socket') as socket_mock:
                wait_for_port('2001:db8::2', 22, closed=False)
        gai_mock.assert_called_once_with(
            '2001:db8::2', 22, socket.AF_INET6, socket.SOCK_STREAM)
        socket_mock.assert_called_once_with(23, 0, 0)
        connect_mock = socket_mock.return_value.connect
        connect_mock.assert_called_once_with(('2001:db8::2', 22, 0, 0))


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

    def test_no_args(self):
        cmd_line = []
        with patch('utility.os.getenv', return_value=False):
            with patch('utility.os.path.isfile', return_value=True):
                parser = add_basic_testing_arguments(ArgumentParser(),
                                                     deadline=True)
                args = parser.parse_args(cmd_line)
        self.assertEqual(args.env, 'lxd')
        self.assertEqual(args.juju_bin, '/usr/bin/juju')

        self.assertEqual(args.logs, None)

        temp_env_name_arg = args.temp_env_name.split("-")
        temp_env_name_ts = temp_env_name_arg[1]
        self.assertEqual(temp_env_name_arg[0:1], ['testutility'])
        self.assertTrue(temp_env_name_ts,
                        datetime.strptime(temp_env_name_ts, "%Y%m%d%H%M%S"))
        self.assertEqual(temp_env_name_arg[2:4], ['temp', 'env'])
        self.assertIs(None, args.deadline)

    def test_no_default_juju_binary(self):
        cmd_line = []
        with self.assertRaises(ValueError):
            with patch('utility.os.getenv', return_value=False):
                with patch('utility.os.path.isfile', return_value=False):
                    parser = add_basic_testing_arguments(ArgumentParser(),
                                                         deadline=True)
                    parser.parse_args(cmd_line)

    def test_juju_binary_snap(self):
        def juju_snap_path(path):
            if path == '/snap/bin/juju':
                return True
            return False

        cmd_line = []
        with patch('utility.os.getenv', return_value=False):
            with patch('utility.os.path.isfile', side_effect=juju_snap_path):
                parser = add_basic_testing_arguments(ArgumentParser(),
                                                     deadline=True)
                args = parser.parse_args(cmd_line)
        self.assertEqual(args.juju_bin, '/snap/bin/juju')

    def test_default_binary_gopath(self):
        cmd_line = []
        with patch('utility.os.getenv', return_value='/tmp'):
            with patch('utility.os.path.isfile', return_value=True):
                parser = add_basic_testing_arguments(ArgumentParser())
                args = parser.parse_args(cmd_line)
        self.assertEqual(args.juju_bin, '/tmp/bin/juju')

    def test_positional_args(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest']
        parser = add_basic_testing_arguments(ArgumentParser(), deadline=True)
        args = parser.parse_args(cmd_line)
        expected = Namespace(
            agent_url=None, debug=False, env='local', temp_env_name='testtest',
            juju_bin='/foo/juju', logs='/tmp/logs', series=None,
            verbose=logging.INFO, agent_stream=None, keep_env=False,
            upload_tools=False, bootstrap_host=None, machine=[], region=None,
            deadline=None)
        self.assertEqual(args, expected)

    def test_positional_args_add_juju_bin_name(self):
        cmd_line = ['local', '/juju', '/tmp/logs', 'testtest']
        parser = add_basic_testing_arguments(ArgumentParser(), deadline=True)
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.juju_bin, '/juju')

    def test_positional_args_accepts_juju_exe(self):
        cmd_line = ['local', 'c:\\juju.exe', '/tmp/logs', 'testtest']
        parser = add_basic_testing_arguments(ArgumentParser(), deadline=True)
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.juju_bin, 'c:\\juju.exe')

    def test_warns_on_dirty_logs(self):
        with warnings.catch_warnings(record=True) as warned:
            with temp_dir() as log_dir:
                open(os.path.join(log_dir, "existing.log"), "w").close()
                cmd_line = ['local', '/a/juju', log_dir, 'testtest']
                parser = add_basic_testing_arguments(ArgumentParser())
                parser.parse_args(cmd_line)
            self.assertEqual(len(warned), 1)
            self.assertRegexpMatches(
                str(warned[0].message),
                r"^Directory '.*' has existing contents.$")
        self.assertEqual("", self.log_stream.getvalue())

    def test_no_warn_on_empty_logs(self):
        """Special case a file named 'empty' doesn't make log dir dirty"""
        with warnings.catch_warnings(record=True) as warned:
            with temp_dir() as log_dir:
                open(os.path.join(log_dir, "empty"), "w").close()
                cmd_line = ['local', '/a/juju', log_dir, 'testtest']
                parser = add_basic_testing_arguments(ArgumentParser())
                parser.parse_args(cmd_line)
            self.assertEqual(warned, [])
        self.assertEqual("", self.log_stream.getvalue())

    def test_no_warn_on_help(self):
        """Special case help should not generate a warning"""
        with warnings.catch_warnings(record=True) as warned:
            with patch('utility.sys.exit'):
                parser = add_basic_testing_arguments(ArgumentParser())
                cmd_line = ['-h']
                parser.parse_args(cmd_line)
                cmd_line = ['--help']
                parser.parse_args(cmd_line)

            self.assertEqual(warned, [])

    def test_warn_on_nonexistent_directory_creation(self):
        with warnings.catch_warnings(record=True) as warned:
            log_dir = mkdtemp()
            os.rmdir(log_dir)
            cmd_line = ['local', '/foo/juju', log_dir, 'testtest']
            parser = add_basic_testing_arguments(ArgumentParser())
            parser.parse_args(cmd_line)
            self.assertEqual(len(warned), 1)
            self.assertRegexpMatches(
                str(warned[0].message),
                r"Not a directory " + log_dir)
            self.assertEqual("", self.log_stream.getvalue())

    def test_debug(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest', '--debug']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.debug, True)

    def test_verbose_logging(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest', '--verbose']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.verbose, logging.DEBUG)

    def test_agent_url(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--agent-url', 'http://example.org']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.agent_url, 'http://example.org')

    def test_agent_stream(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--agent-stream', 'testing']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.agent_stream, 'testing')

    def test_series(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest', '--series',
                    'vivid']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.series, 'vivid')

    def test_upload_tools(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--upload-tools']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertTrue(args.upload_tools)

    def test_using_jes_upload_tools(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--upload-tools']
        parser = add_basic_testing_arguments(ArgumentParser(), using_jes=True)
        with patch.object(parser, 'error') as mock_error:
            parser.parse_args(cmd_line)
        mock_error.assert_called_once_with(
            'unrecognized arguments: --upload-tools')

    def test_bootstrap_host(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--bootstrap-host', 'bar']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.bootstrap_host, 'bar')

    def test_machine(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--machine', 'bar', '--machine', 'baz']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.machine, ['bar', 'baz'])

    def test_keep_env(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--keep-env']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertTrue(args.keep_env)

    def test_region(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest',
                    '--region', 'foo-bar']
        parser = add_basic_testing_arguments(ArgumentParser())
        args = parser.parse_args(cmd_line)
        self.assertEqual('foo-bar', args.region)

    def test_deadline(self):
        now = datetime(2012, 11, 10, 9, 8, 7)
        cmd_line = ['--timeout', '300']
        parser = add_basic_testing_arguments(ArgumentParser(), deadline=True)
        with patch('utility.datetime') as dt_class:
            # Can't patch the utcnow method of datetime.datetime (because it's
            # C code?) but we can patch out the whole datetime class.
            dt_class.utcnow.return_value = now
            args = parser.parse_args(cmd_line)
        self.assertEqual(now + timedelta(seconds=300), args.deadline)

    def test_no_env(self):
        cmd_line = ['/foo/juju', '/tmp/logs', 'testtest']
        parser = add_basic_testing_arguments(ArgumentParser(), env=False)
        args = parser.parse_args(cmd_line)
        expected = Namespace(
            agent_url=None, debug=False, temp_env_name='testtest',
            juju_bin='/foo/juju', logs='/tmp/logs', series=None,
            verbose=logging.INFO, agent_stream=None, keep_env=False,
            upload_tools=False, bootstrap_host=None, machine=[], region=None,
            deadline=None)
        self.assertEqual(args, expected)


class TestSkipOnMissingFile(TestCase):

    def test_skip_on_missing_file(self):
        """Test if skip_on_missing_file hides the proper exceptions."""
        with skip_on_missing_file():
            raise OSError(errno.ENOENT, 'should be hidden')
        with skip_on_missing_file():
            raise IOError(errno.ENOENT, 'should be hidden')

    def test_skip_on_missing_file_except(self):
        """Test if skip_on_missing_file ignores other types of exceptions."""
        with self.assertRaises(RuntimeError):
            with skip_on_missing_file():
                raise RuntimeError(errno.ENOENT, 'pass through')
        with self.assertRaises(IOError):
            with skip_on_missing_file():
                raise IOError(errno.EEXIST, 'pass through')


class TestRunCommand(TestCase):

    def test_run_command_args(self):
        with patch('subprocess.check_output') as co_mock:
            run_command(['foo', 'bar'])
        args, kwargs = co_mock.call_args
        self.assertEqual((['foo', 'bar'], ), args)

    def test_run_command_dry_run(self):
        with patch('subprocess.check_output') as co_mock:
            run_command(['foo', 'bar'], dry_run=True)
            self.assertEqual(0, co_mock.call_count)

    def test_run_command_verbose(self):
        with patch('subprocess.check_output'):
            with patch('utility.print_now') as p_mock:
                run_command(['foo', 'bar'], verbose=True)
                self.assertEqual(2, p_mock.call_count)


class TestQuote(TestCase):

    def test_quote(self):
        self.assertEqual(quote("arg"), "arg")
        self.assertEqual(quote("/a/file name"), "'/a/file name'")
        self.assertEqual(quote("bob's"), "'bob'\"'\"'s'")


class TestGetWinRmCerts(TestCase):

    def test_get_certs(self):
        with patch.dict(os.environ, {"HOME": "/fake/home"}):
            certs = get_winrm_certs()
        self.assertEqual(certs, (
            "/fake/home/cloud-city/winrm_client_cert.key",
            "/fake/home/cloud-city/winrm_client_cert.pem",
        ))


class TestScopedEnviron(TestCase):

    def test_scoped_environ(self):
        old_environ = dict(os.environ)
        with scoped_environ():
            os.environ.clear()
            os.environ['foo'] = 'bar'
            self.assertNotEqual(old_environ, os.environ)
        self.assertEqual(old_environ, os.environ)

    def test_new_environ(self):
        new_environ = {'foo': 'bar'}
        with scoped_environ(new_environ):
            self.assertEqual(os.environ, new_environ)
        self.assertNotEqual(os.environ, new_environ)


class TestTempDir(TestCase):

    def test_temp_dir(self):
        with temp_dir() as d:
            self.assertTrue(os.path.isdir(d))
        self.assertFalse(os.path.exists(d))

    def test_temp_dir_contents(self):
        with temp_dir() as d:
            self.assertTrue(os.path.isdir(d))
            open(os.path.join(d, "a-file"), "w").close()
        self.assertFalse(os.path.exists(d))

    def test_temp_dir_parent(self):
        with temp_dir() as p:
            with temp_dir(parent=p) as d:
                self.assertTrue(os.path.isdir(d))
                self.assertEqual(p, os.path.dirname(d))
            self.assertFalse(os.path.exists(d))
        self.assertFalse(os.path.exists(p))

    def test_temp_dir_keep(self):
        with temp_dir() as p:
            with temp_dir(parent=p, keep=True) as d:
                self.assertTrue(os.path.isdir(d))
                open(os.path.join(d, "a-file"), "w").close()
            self.assertTrue(os.path.exists(d))
            self.assertTrue(os.path.exists(os.path.join(d, "a-file")))
        self.assertFalse(os.path.exists(p))


class TestUnqualifiedModelName(TestCase):

    def test_returns_just_model_name_when_passed_qualifed_full_username(self):
        self.assertEqual(
            unqualified_model_name('admin@local/default'),
            'default'
        )

    def test_returns_just_model_name_when_passed_just_username(self):
        self.assertEqual(
            unqualified_model_name('admin/default'),
            'default'
        )

    def test_returns_just_model_name_when_passed_no_username(self):
        self.assertEqual(
            unqualified_model_name('default'),
            'default'
        )


class TestQualifiedModelName(TestCase):

    def test_raises_valueerror_when_model_name_blank(self):
        with self.assertRaises(ValueError):
            qualified_model_name('', 'admin@local')

    def test_raises_valueerror_when_owner_name_blank(self):
        with self.assertRaises(ValueError):
            qualified_model_name('default', '')

    def test_raises_valueerror_when_owner_and_model_blank(self):
        with self.assertRaises(ValueError):
            qualified_model_name('', '')

    def test_raises_valueerror_when_owner_name_doesnt_match_model_owner(self):
        with self.assertRaises(ValueError):
            qualified_model_name('test/default', 'admin')

        with self.assertRaises(ValueError):
            qualified_model_name('test@local/default', 'admin@local')

    def test_returns_qualified_model_name_with_plain_model_name(self):
        self.assertEqual(
            qualified_model_name('default', 'admin@local'),
            'admin@local/default'
        )

        self.assertEqual(
            qualified_model_name('default', 'admin'),
            'admin/default'
        )

    def test_returns_qualified_model_name_with_model_name_with_owner(self):
        self.assertEqual(
            qualified_model_name('admin@local/default', 'admin@local'),
            'admin@local/default'
        )

        self.assertEqual(
            qualified_model_name('admin/default', 'admin'),
            'admin/default'
        )


class TestLogAndWrapException(TestCase):

    def test_exception(self):
        mock_logger = Mock(spec=['exception'])
        err = Exception('an error')
        wrapped = log_and_wrap_exception(mock_logger, err)
        self.assertIs(wrapped.exception, err)
        mock_logger.exception.assert_called_once_with(err)

    def test_has_stdout(self):
        mock_logger = Mock(spec=['exception', 'info'])
        err = Exception('another error')
        err.output = 'stdout text'
        wrapped = log_and_wrap_exception(mock_logger, err)
        self.assertIs(wrapped.exception, err)
        mock_logger.exception.assert_called_once_with(err)
        mock_logger.info.assert_called_once_with(
            'Output from exception:\nstdout:\n%s\nstderr:\n%s', 'stdout text',
            None)

    def test_has_stderr(self):
        mock_logger = Mock(spec=['exception', 'info'])
        err = Exception('another error')
        err.stderr = 'stderr text'
        wrapped = log_and_wrap_exception(mock_logger, err)
        self.assertIs(wrapped.exception, err)
        mock_logger.exception.assert_called_once_with(err)
        mock_logger.info.assert_called_once_with(
            'Output from exception:\nstdout:\n%s\nstderr:\n%s', None,
            'stderr text')


class TestLoggedException(TestCase):

    def test_no_error_no_log(self):
        mock_logger = Mock(spec_set=[])
        with logged_exception(mock_logger):
            pass

    def test_exception_logged_and_wrapped(self):
        mock_logger = Mock(spec=['exception'])
        err = Exception('some error')
        with self.assertRaises(LoggedException) as ctx:
            with logged_exception(mock_logger):
                raise err
        self.assertIs(ctx.exception.exception, err)
        mock_logger.exception.assert_called_once_with(err)

    def test_exception_logged_once(self):
        mock_logger = Mock(spec=['exception'])
        err = Exception('another error')
        with self.assertRaises(LoggedException) as ctx:
            with logged_exception(mock_logger):
                with logged_exception(mock_logger):
                    raise err
        self.assertIs(ctx.exception.exception, err)
        mock_logger.exception.assert_called_once_with(err)

    def test_generator_exit_not_wrapped(self):
        mock_logger = Mock(spec_set=[])
        with self.assertRaises(GeneratorExit):
            with logged_exception(mock_logger):
                raise GeneratorExit

    def test_keyboard_interrupt_wrapped(self):
        mock_logger = Mock(spec=['exception'])
        err = KeyboardInterrupt()
        with self.assertRaises(LoggedException) as ctx:
            with logged_exception(mock_logger):
                raise err
        self.assertIs(ctx.exception.exception, err)
        mock_logger.exception.assert_called_once_with(err)

    def test_output_logged(self):
        mock_logger = Mock(spec=['exception', 'info'])
        err = Exception('some error')
        err.output = 'some output'
        with self.assertRaises(LoggedException) as ctx:
            with logged_exception(mock_logger):
                raise err
        self.assertIs(ctx.exception.exception, err)
        mock_logger.exception.assert_called_once_with(err)
        mock_logger.info.assert_called_once_with(
            'Output from exception:\nstdout:\n%s\nstderr:\n%s', 'some output',
            None)
