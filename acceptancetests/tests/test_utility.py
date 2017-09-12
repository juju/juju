from argparse import (
    ArgumentParser,
    Namespace,
    )
from datetime import (
    datetime,
    timedelta,
    )
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

from jujupy.utility import (
    temp_dir,
    )
from tests import (
    TestCase,
    )
from utility import (
    add_basic_testing_arguments,
    assert_dict_is_subset,
    as_literal_address,
    extract_deb,
    _find_candidates,
    find_candidates,
    find_latest_branch_candidates,
    get_candidates_path,
    get_deb_arch,
    get_winrm_certs,
    JujuAssertionError,
    log_and_wrap_exception,
    logged_exception,
    LoggedException,
    run_command,
    wait_for_port,
    )


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
        parser = add_basic_testing_arguments(ArgumentParser(),
                                             deadline=True)
        args = parser.parse_args(cmd_line)
        self.assertEqual(args.env, 'lxd')
        self.assertEqual(args.juju_bin, None)

        self.assertEqual(args.logs, None)

        temp_env_name_arg = args.temp_env_name.split("-")
        temp_env_name_ts = temp_env_name_arg[1]
        self.assertEqual(temp_env_name_arg[0:1], ['testutility'])
        self.assertTrue(temp_env_name_ts,
                        datetime.strptime(temp_env_name_ts, "%Y%m%d%H%M%S"))
        self.assertEqual(temp_env_name_arg[2:4], ['temp', 'env'])
        self.assertIs(None, args.deadline)

    def test_positional_args(self):
        cmd_line = ['local', '/foo/juju', '/tmp/logs', 'testtest']
        parser = add_basic_testing_arguments(ArgumentParser(), deadline=True)
        args = parser.parse_args(cmd_line)
        expected = Namespace(
            agent_url=None, debug=False, env='local', temp_env_name='testtest',
            juju_bin='/foo/juju', logs='/tmp/logs', series=None,
            verbose=logging.INFO, agent_stream=None, keep_env=False,
            upload_tools=False, bootstrap_host=None, machine=[], region=None,
            deadline=None, to=None, existing=None)
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
            deadline=None, to=None, existing=None)
        self.assertEqual(args, expected)


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


class TestGetWinRmCerts(TestCase):

    def test_get_certs(self):
        with patch.dict(os.environ, {"HOME": "/fake/home"}):
            certs = get_winrm_certs()
        self.assertEqual(certs, (
            "/fake/home/cloud-city/winrm_client_cert.key",
            "/fake/home/cloud-city/winrm_client_cert.pem",
        ))


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


class TestAssertDictIsSubset(TestCase):

    def test_assert_dict_is_subset(self):
        # Identical dicts.
        self.assertIsTrue(
            assert_dict_is_subset(
                {'a': 1, 'b': 2},
                {'a': 1, 'b': 2}))
        # super dict has an extra item.
        self.assertIsTrue(
            assert_dict_is_subset(
                {'a': 1, 'b': 2},
                {'a': 1, 'b': 2, 'c': 3}))
        # A key is missing.
        with self.assertRaises(JujuAssertionError):
            assert_dict_is_subset(
                {'a': 1, 'b': 2},
                {'a': 1, 'c': 2})
        # A value is different.
        with self.assertRaises(JujuAssertionError):
            assert_dict_is_subset(
                {'a': 1, 'b': 2},
                {'a': 1, 'b': 4})
