"""Tests for assess_constraints module."""

import logging
from mock import Mock, patch
import StringIO
import os
from contextlib import contextmanager

from assess_constraints import (
    Constraints,
    assess_virt_type_constraints,
    assess_instance_type_constraints,
    deploy_charm_constraint,
    parse_args,
    main,
    INSTANCE_TYPES,
    )
from tests import (
    parse_error,
    TestCase,
    )
from tests.test_jujupy import fake_juju_client
from utility import (
    JujuAssertionError,
    temp_dir,
    )


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


class TestConstraints(TestCase):

    def test_static_str(self):
        string = Constraints.str(mem='2G', root_disk='4G', virt_type='lxd')
        self.assertEqual('mem=2G virt-type=lxd root-disk=4G', string)

    def test_static_str_none(self):
        self.assertEqual('', Constraints.str())
        self.assertEqual('', Constraints.str(arch=None))

    def test_str_operator(self):
        constraints = Constraints(mem='2G', root_disk='4G', virt_type='lxd')
        self.assertEqual('mem=2G virt-type=lxd root-disk=4G',
                         str(constraints))


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_constraints.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_constraints.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_constraints.assess_constraints",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, False)


class TestAssess(TestCase):

    @contextmanager
    def prepare_deploy_mock(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        """Mock a client and the deploy function."""
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        with patch('jujupy.EnvJujuClient.deploy',
                   autospec=True) as deploy_mock:
            yield fake_client, deploy_mock

    def gather_constraint_args(self, mock):
        """Create a list of the constraint arguments passed to the mock."""
        constraint_args = [
            args[1]["constraints"] for args in mock.call_args_list]
        return constraint_args

    def test_virt_type_constraints_with_kvm(self):
        assert_constraints_calls = ["virt-type=lxd", "virt-type=kvm"]
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            assess_virt_type_constraints(fake_client, True)
        constraints_calls = self.gather_constraint_args(deploy_mock)
        self.assertEqual(constraints_calls, assert_constraints_calls)

    def test_virt_type_constraints_without_kvm(self):
        assert_constraints_calls = ["virt-type=lxd"]
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            assess_virt_type_constraints(fake_client, False)
        constraints_calls = self.gather_constraint_args(deploy_mock)
        self.assertEqual(constraints_calls, assert_constraints_calls)

    def test_instance_type_constraints(self):
        assert_constraints_calls = ['instance-type=bar', 'instance-type=baz']
        fake_instance_types = ['bar', 'baz']
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            fake_provider = fake_client.env.config.get('type')
            with patch.dict(INSTANCE_TYPES,
                            {fake_provider: fake_instance_types}):
                assess_instance_type_constraints(fake_client)
        constraints_calls = self.gather_constraint_args(deploy_mock)
        self.assertEqual(constraints_calls, assert_constraints_calls)

    def test_instance_type_constraints_missing(self):
        fake_client = Mock(wraps=fake_juju_client())
        with self.assertRaises(ValueError):
            assess_instance_type_constraints(fake_client)


class TestDeploy(TestCase):

    def test_deploy_charm_constraint(self):
        fake_client = Mock(wraps=fake_juju_client())
        charm_name = 'test-constraint'
        charm_series = 'xenial'
        constraints = 'mem=10GB'
        with temp_dir() as charm_dir:
            with patch('assess_constraints.deploy_constraint',
                       autospec=True) as deploy_mock:
                deploy_charm_constraint(fake_client, constraints, charm_name,
                                        charm_series, charm_dir)
        charm = os.path.join(charm_dir, charm_series, charm_name)
        deploy_mock.assert_called_once_with(fake_client, constraints, charm,
                                            charm_series, charm_dir)
