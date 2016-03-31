"""Tests for assess_min_version module."""

import logging
from mock import (
    Mock,
    patch,
    call,
)
import os
import StringIO
import subprocess

import yaml

from assess_min_version import (
    assess_deploy,
    assert_fail,
    assess_min_version,
    assert_pass,
    get_current_version,
    JujuAssertionError,
    make_charm,
    main,
    parse_args,
)
from tests import (
    parse_error,
    TestCase,
)
from utility import temp_dir


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_min_version.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_min_version.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_min_version.assess_min_version",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, parse_args(argv))


class TestAssess(TestCase):

    def test_assert_fail(self):
        mock_client = Mock(spec=["deploy"])
        mock_client.deploy.side_effect = subprocess.CalledProcessError('', '')
        assert_fail(mock_client, "dummpy", "2.0", "2.0", "name")

    def test_assert_fail_exception(self):
        mock_client = Mock(spec=["deploy"])
        with self.assertRaisesRegexp(
                JujuAssertionError, 'assert_fail failed min: 2.0 cur: 2.0'):
            assert_fail(mock_client, "dummpy", "2.0", "2.0", "name")

    def test_assert_pass(self):
        mock_client = Mock(spec=["deploy", "wait_for_started"])
        assert_pass(mock_client, "dummpy", "2.0", "2.0", "name")

    def test_assert_pass_exception(self):
        mock_client = Mock(spec=["deploy", "wait_for_started"])
        mock_client.deploy.side_effect = subprocess.CalledProcessError('', '')
        with self.assertRaisesRegexp(
                JujuAssertionError, 'assert_pass failed min: 2.0 cur: 2.0'):
            assert_pass(mock_client, "dummpy", "2.0", "2.0", "name")

    def test_make_charm(self):
        with temp_dir() as charm_dir:
            make_charm(charm_dir, "2.0", name="foo")
            metadata = os.path.join(charm_dir, 'metadata.yaml')
            with open(metadata, 'r') as f:
                content = yaml.load(f)
        self.assertEqual(content['name'], 'foo')
        self.assertEqual(content['min-juju-version'], '2.0')
        self.assertEqual(content['summary'], 'summary')

    def test_get_current_version(self):
        mock_client = Mock(spec=["get_version"])
        mock_client.get_version.return_value = '2.0-beta4-trusty-amd64'
        ver = get_current_version(mock_client, '/tmp/bin')
        self.assertEqual(ver, '2.0-beta4')

        mock_client.get_version.return_value = '1.25.4-trusty-amd64'
        ver = get_current_version(mock_client, '/tmp/bin')
        self.assertEqual(ver, '1.25.4')

    def test_assess_deploy(self):
        mock_client = Mock(spec=["deploy", "wait_for_started"])
        mock_assertion = Mock(spec=[])
        with patch("assess_min_version.temp_dir", autospec=True) as mock_td:
            with patch("assess_min_version.make_charm",
                       autospec=True) as mock_mc:
                assess_deploy(
                    mock_client, mock_assertion, "2.1", "2.0", "dummy")
        temp_dir = mock_td.return_value.__enter__.return_value
        mock_assertion.assert_called_once_with(
            mock_client, temp_dir, "2.1", "2.0", "dummy")
        mock_mc.assert_called_once_with(temp_dir, "2.1")

    def test_assess_min_version(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        args = parse_args(argv)
        mock_client = Mock(spec=["juju", "wait_for_started"])
        with patch("assess_min_version.get_current_version",
                   autospec=True, return_value="2.0.0") as mock_gcv:
            with patch("assess_min_version.assess_deploy",
                       autospec=True) as mock_ad:
                assess_min_version(mock_client, args)
        mock_gcv.assert_called_once_with(mock_client, '/bin/juju')
        ad_calls = [
            call(mock_client, assert_pass, '1.25.0', '2.0.0', 'name1250'),
            call(mock_client, assert_fail, '99.9.9', '2.0.0', 'name9999'),
            call(mock_client, assert_fail, '99.9-alpha1', '2.0.0',
                 'name999alpha1'),
            call(mock_client, assert_pass, '1.2-beta1', '2.0.0',
                 'name12beta1'),
            call(mock_client, assert_pass, '1.25.5.1', '2.0.0', 'name12551'),
            call(mock_client, assert_pass, '2.0-alpha1', '2.0.0',
                 'name20alpha1'),
            call(mock_client, assert_pass, '2.0.0', '2.0.0', 'current')]
        self.assertEqual(mock_ad.mock_calls, ad_calls)
