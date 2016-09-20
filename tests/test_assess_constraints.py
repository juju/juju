"""Tests for assess_constraints module."""

from contextlib import contextmanager
import logging
from mock import call, Mock, patch
import os
import StringIO

from assess_constraints import (
    assess_virt_type_constraints,
    assess_instance_type_constraints,
    Constraints,
    deploy_constraint,
    deploy_charm_constraint,
    juju_show_machine_hardware,
    parse_args,
    main,
    mem_to_int,
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

    def test_mem_to_int(self):
        self.assertEqual(1, mem_to_int('1'))
        self.assertEqual(1, mem_to_int('1M'))
        self.assertEqual(1024, mem_to_int('1G'))
        self.assertEqual(4096, mem_to_int('4G'))
        with self.assertRaises(JujuAssertionError):
            mem_to_int('40XB')

    def test_str_operator(self):
        constraints = Constraints(mem='2G', root_disk='4G', virt_type='lxd')
        self.assertEqual('mem=2G virt-type=lxd root-disk=4G',
                         str(constraints))

    def test_str_operator_none(self):
        self.assertEqual('', str(Constraints()))
        self.assertEqual('', str(Constraints(arch=None)))

    def test__meets_string(self):
        meets_string = Constraints._meets_string
        self.assertTrue(meets_string(None, 'amd64'))
        self.assertTrue(meets_string('amd64', 'amd64'))
        self.assertFalse(meets_string('i32', 'amd64'))

    def test__meets_min_int(self):
        meets_min_int = Constraints._meets_min_int
        self.assertTrue(meets_min_int(None, '2'))
        self.assertFalse(meets_min_int('2', '1'))
        self.assertTrue(meets_min_int('2', '2'))
        self.assertTrue(meets_min_int('2', '3'))

    def test__meets_min_mem(self):
        meets_min_mem = Constraints._meets_min_mem
        self.assertTrue(meets_min_mem(None, '64'))
        self.assertFalse(meets_min_mem('1G', '512M'))
        self.assertTrue(meets_min_mem('1G', '1024'))
        self.assertTrue(meets_min_mem('1G', '1G'))
        self.assertTrue(meets_min_mem('1G', '2G'))

    def test_meets_instance_type(self):
        constraints = Constraints(instance_type='t2.micro')
        data1 = {'mem': '1G', 'cpu-power': '10', 'cores': '1'}
        self.assertTrue(constraints.meets_instance_type(data1))
        data2 = {'mem': '8G', 'cpu-power': '20', 'cores': '1'}
        self.assertFalse(constraints.meets_instance_type(data2))
        data3 = dict(data1, arch='amd64')
        self.assertTrue(constraints.meets_instance_type(data3))
        data4 = {'root-disk': '1G', 'cpu-power': '10', 'cores': '1'}
        with self.assertRaises(JujuAssertionError):
            constraints.meets_instance_type(data4)

    def test_meets_instance_type_fix(self):
        constraints = Constraints(instance_type='t2.micro')
        data = {'mem': '1G', 'cpu-power': '10', 'cpu-cores': '1'}
        self.assertTrue(constraints.meets_instance_type(data))


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

    @contextmanager
    def patch_hardware(self, return_values):
        """Patch juju_show_machine_hardware with a series of return values."""
        def pop_hardware(client, machine):
            return return_values.pop(0)
        with patch('assess_constraints.juju_show_machine_hardware',
                   autospec=True, side_effect=pop_hardware) as hardware_mock:
            yield hardware_mock

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

    @contextmanager
    def patch_instance_spec(self, fake_provider, passing=True):
        fake_instance_types = ['bar', 'baz']
        bar_data = {'cpu-power': '20'}
        baz_data = {'cpu-power': '30'}

        def mock_get_instance_spec(instance_type):
            if 'bar' == instance_type:
                return bar_data
            elif 'baz' == instance_type and passing:
                return baz_data
            elif 'baz' == instance_type:
                return {'cpu-power': '40'}
            else:
                raise ValueError('instance-type not in mock.')
        with patch.dict(INSTANCE_TYPES, {fake_provider: fake_instance_types}):
            with patch('assess_constraints.get_instance_spec', autospec=True,
                       side_effect=mock_get_instance_spec) as spec_mock:
                with self.patch_hardware([bar_data, baz_data]):
                    yield spec_mock

    def test_instance_type_constraints(self):
        assert_constraints_calls = ['instance-type=bar', 'instance-type=baz']
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            fake_provider = fake_client.env.config.get('type')
            with self.patch_instance_spec(fake_provider) as spec_mock:
                assess_instance_type_constraints(fake_client)
        constraints_calls = self.gather_constraint_args(deploy_mock)
        self.assertEqual(constraints_calls, assert_constraints_calls)
        self.assertEqual(spec_mock.call_args_list, [call('bar'), call('baz')])

    def test_instance_type_constraints_fail(self):
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            fake_provider = fake_client.env.config.get('type')
            with self.patch_instance_spec(fake_provider, False) as spec_mock:
                with self.assertRaises(ValueError):
                    assess_instance_type_constraints(fake_client)

    def test_instance_type_constraints_missing(self):
        fake_client = Mock(wraps=fake_juju_client())
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            assess_instance_type_constraints(fake_client)
        self.assertFalse(deploy_mock.called)


class TestDeploy(TestCase):

    def test_deploy_constraint(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.attach_mock(Mock(), 'deploy')
        fake_client.attach_mock(Mock(), 'wait_for_workloads')
        charm_name = 'test-constraint'
        charm_series = 'xenial'
        constraints = Constraints(mem='10GB')
        with temp_dir() as charm_dir:
            charm = os.path.join(charm_dir, charm_series, charm_name)
            deploy_constraint(fake_client, constraints, charm, charm_series,
                              charm_dir)
        fake_client.deploy.assert_called_once_with(
            charm, series=charm_series, repository=charm_dir,
            constraints=str(constraints))
        fake_client.wait_for_workloads.assert_called_once_with()

    def test_deploy_charm_constraint(self):
        fake_client = Mock(wraps=fake_juju_client())
        charm_name = 'test-constraint'
        charm_series = 'xenial'
        constraints = Constraints(mem='10GB')
        with temp_dir() as charm_dir:
            with patch('assess_constraints.deploy_constraint',
                       autospec=True) as deploy_mock:
                deploy_charm_constraint(fake_client, constraints, charm_name,
                                        charm_series, charm_dir)
        charm = os.path.join(charm_dir, charm_series, charm_name)
        deploy_mock.assert_called_once_with(fake_client, constraints, charm,
                                            charm_series, charm_dir)


class TestJujuWrappers(TestCase):

    SAMPLE_JUJU_SHOW_MACHINE_OUTPUT = """\
model:
  name: controller
  controller: assessconstraints-20160914122952-temp-env
  cloud: lxd
  region: localhost
  version: 2.0-beta18
machines:
  "0":
    juju-status:
      current: started
      since: 14 Sep 2016 12:32:17-04:00
      version: 2.0-beta18
    dns-name: 10.252.22.39
    instance-id: juju-7d249e-0
    machine-status:
      current: pending
      since: 14 Sep 2016 12:32:07-04:00
    series: xenial
    hardware: arch=amd64 cpu-cores=0 mem=0M
    controller-member-status: has-vote
applications: {}"""

    def test_juju_show_machine_hardware(self):
        """Check the juju_show_machine_hardware data translation."""
        output_mock = Mock(
            return_value=self.SAMPLE_JUJU_SHOW_MACHINE_OUTPUT)
        fake_client = Mock(get_juju_output=output_mock)
        data = juju_show_machine_hardware(fake_client, '0')
        output_mock.assert_called_once_with('show-machine', '0',
                                            '--format', 'yaml')
        self.assertEqual({'arch': 'amd64', 'cpu-cores': '0', 'mem': '0M'},
                         data)
