"""Tests for assess_constraints module."""

import logging
from mock import Mock, patch
import StringIO

from assess_constraints import (
    assess_constraints,
    parse_args,
    main,
)
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import fake_juju_client


class TestParseArgs(TestCase):

    def test_common_args(self):
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
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_constraints.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_constraints.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_c:
                    with patch("assess_constraints.assess_constraints",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                       soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, False)


class TestAssess(TestCase):

    def test_constraints_with_kvm(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        fake_client = Mock(wraps=fake_juju_client())
        assert_constraints_calls = ["virt-type=lxd", "virt-type=kvm"]
        fake_client.bootstrap()
        deploy = patch('jujupy.EnvJujuClient.deploy',
                       autospec=True)
        with deploy as deploy_mock:
            assess_constraints(fake_client, True)
        constraints_calls = [
            call[1]["constraints"] for call in
            deploy_mock.call_args_list]
        self.assertEqual(constraints_calls, assert_constraints_calls)

    def test_constraints_without_kvm(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        fake_client = Mock(wraps=fake_juju_client())
        assert_constraints_calls = ["virt-type=lxd"]
        fake_client.bootstrap()
        deploy = patch('jujupy.EnvJujuClient.deploy',
                       autospec=True)
        with deploy as deploy_mock:
            assess_constraints(fake_client, False)
        constraints_calls = [
            call[1]["constraints"] for call in
            deploy_mock.call_args_list]
        self.assertEqual(constraints_calls, assert_constraints_calls)
