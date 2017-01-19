"""Tests for assess_constraints module."""

from contextlib import contextmanager
import logging
from mock import call, Mock, patch
import os
import StringIO

from assess_constraints import (
    application_hardware,
    application_machines,
    assess_constraints_deploy,
    assess_cores_constraints,
    assess_cpu_power_constraints,
    assess_instance_type_constraints,
    assess_multiple_constraints,
    assess_root_disk_constraints,
    assess_virt_type_constraints,
    Constraints,
    deploy_constraint,
    deploy_charm_constraint,
    machine_hardware,
    parse_args,
    main,
    mem_to_int,
    INSTANCE_TYPES,
    )
from jujupy import (
    fake_juju_client,
    Status,
    )
from tests import (
    parse_error,
    TestCase,
    )
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

    def test_repr_operator(self):
        self.assertEqual("Constraints()", repr(Constraints()))
        constraints = Constraints(root_disk='4G', mem='2G')
        self.assertEqual("Constraints(mem='2G', root_disk='4G')",
                         repr(constraints))

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
        self.assertFalse(meets_string('amd64', None))
        self.assertTrue(meets_string('amd64', 'amd64'))
        self.assertFalse(meets_string('arm64', 'amd64'))

    def test__meets_min_int(self):
        meets_min_int = Constraints._meets_min_int
        self.assertTrue(meets_min_int(None, '2'))
        self.assertFalse(meets_min_int('2', None))
        self.assertFalse(meets_min_int('2', '1'))
        self.assertTrue(meets_min_int('2', '2'))
        self.assertTrue(meets_min_int('2', '3'))

    def test__meets_min_mem(self):
        meets_min_mem = Constraints._meets_min_mem
        self.assertTrue(meets_min_mem(None, '64'))
        self.assertFalse(meets_min_mem('1G', None))
        self.assertFalse(meets_min_mem('1G', '512M'))
        self.assertTrue(meets_min_mem('1G', '1024'))
        self.assertTrue(meets_min_mem('1G', '1G'))
        self.assertTrue(meets_min_mem('1G', '2G'))

    def test_meets_root_disk(self):
        constraints = Constraints(root_disk='8G')
        self.assertTrue(constraints.meets_root_disk('8G'))
        self.assertFalse(constraints.meets_root_disk('4G'))

    def test_meets_cores(self):
        constraints = Constraints(cores='2')
        self.assertTrue(constraints.meets_cores('3'))
        self.assertFalse(constraints.meets_cores('1'))

    def test_meets_cpu_power(self):
        constraints = Constraints(cpu_power='20')
        self.assertTrue(constraints.meets_cpu_power('30'))
        self.assertFalse(constraints.meets_cpu_power('10'))

    def test_meets_arch(self):
        constraints = Constraints(arch='amd64')
        self.assertTrue(constraints.meets_arch('amd64'))
        self.assertFalse(constraints.meets_arch('arm64'))

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

    def test_meets_instance_type_none(self):
        constraints = Constraints()
        data = {'mem': '1G', 'cores': '1'}
        self.assertTrue(constraints.meets_instance_type(data))

    def test_meets_all(self):
        constraints = Constraints(cores='2', arch='amd64')
        data1 = {'cores': '2', 'arch': 'amd64'}
        self.assertTrue(constraints.meets_all(data1))
        data2 = {'cores': '1', 'arch': 'amd64'}
        self.assertFalse(constraints.meets_all(data2))


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
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
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
        fake_client = fake_juju_client()
        env = fake_client.env
        fake_client = Mock(wraps=fake_client)
        # force the real env, because attribute access on a wrapped one is
        # weird.
        fake_client.env = env
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
        """Patch machine_hardware with a series of return values.

        Also patches application_machines so application_hardware goes
        straight to machine_hardware."""
        with patch('assess_constraints.machine_hardware',
                   autospec=True, side_effect=return_values) as hardware_mock:
            with patch('assess_constraints.application_machines',
                       autospec=True, return_value=['0']):
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

    def inner_test_constraints_deploy(self, tests_specs):
        """Run a test or series of tests on assess_constraints_deploy.

        :param tests_spec: List of 3 tuples (Constraints args dict, expected
        constraint argument on deploy, return value for data)."""
        constraints_list = [spec[0] for spec in tests_specs]
        expected_call_list = [spec[1] for spec in tests_specs]
        patch_return_list = [spec[2] for spec in tests_specs]
        with self.patch_hardware(patch_return_list):
            with self.prepare_deploy_mock() as (fake_client, deploy_mock):
                for constraints_args in constraints_list:
                    constraints = Constraints(**constraints_args)
                    assess_constraints_deploy(fake_client, constraints,
                                              'tests')
        constraints_calls = self.gather_constraint_args(deploy_mock)
        self.assertEqual(constraints_calls, expected_call_list)

    def test_constraints_deploy(self):
        self.inner_test_constraints_deploy([
            ({'mem': '2G'}, 'mem=2G', {'mem': '2G'}),
            ({'arch': 'amd64'}, 'arch=amd64', {'arch': 'amd64'}),
            ({'cores': '2', 'arch': 'arm64'}, 'cores=2 arch=arm64',
             {'cores': '2', 'arch': 'arm64'}),
            ])

    def test_constraints_deploy_fail(self):
        with self.assertRaises(JujuAssertionError):
            self.inner_test_constraints_deploy(
                [({'cores': '2', 'arch': 'arm64'}, 'cores=2 arch=arm64',
                  {'cores': '1', 'arch': 'arm64'})])

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

    def test_constraints_deploy_instance_type(self):
        constraints_list = [Constraints(instance_type='bar'),
                            Constraints(instance_type='baz')]
        expected_calls = ['instance-type=bar', 'instance-type=baz']
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            fake_provider = fake_client.env.provider
            with self.patch_instance_spec(fake_provider):
                for constraints in constraints_list:
                    assess_constraints_deploy(fake_client, constraints,
                                              'tests')
        constraints_calls = self.gather_constraint_args(deploy_mock)
        self.assertEqual(constraints_calls, expected_calls)

    def test_instance_type_constraints(self):
        assert_constraints_calls = ['instance-type=bar', 'instance-type=baz']
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            fake_provider = fake_client.env.provider
            with self.patch_instance_spec(fake_provider) as spec_mock:
                assess_instance_type_constraints(fake_client)
        constraints_calls = self.gather_constraint_args(deploy_mock)
        self.assertEqual(constraints_calls, assert_constraints_calls)
        self.assertEqual(spec_mock.call_args_list, [call('bar'), call('baz')])

    def test_instance_type_constraints_fail(self):
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            fake_provider = fake_client.env.provider
            with self.patch_instance_spec(fake_provider, False):
                with self.assertRaisesRegexp(
                        JujuAssertionError,
                        'Test Failed: on {} with constraints "{}"'.format(
                            fake_provider, 'instance-type=baz')):
                    assess_instance_type_constraints(fake_client)

    def test_instance_type_constraints_missing(self):
        fake_client = Mock(wraps=fake_juju_client())
        with self.prepare_deploy_mock() as (fake_client, deploy_mock):
            assess_instance_type_constraints(fake_client)
        self.assertFalse(deploy_mock.called)

    def test_root_disk_constraints(self):
        fake_client = Mock(wraps=fake_juju_client())
        with patch('assess_constraints.prepare_constraint_test',
                   autospec=True, return_value={'root-disk': '8G'}
                   ) as prepare_mock:
            with self.assertRaises(JujuAssertionError):
                assess_root_disk_constraints(fake_client, ['8G', '16G'])
        self.assertEqual(2, prepare_mock.call_count)

    def test_cores_constraints(self):
        fake_client = Mock(wraps=fake_juju_client())
        with patch('assess_constraints.prepare_constraint_test',
                   autospec=True, return_value={'cores': '2'}
                   ) as prepare_mock:
            with self.assertRaises(JujuAssertionError):
                assess_cores_constraints(fake_client, ['2', '4'])
        self.assertEqual(2, prepare_mock.call_count)

    def test_cpu_power_constraints(self):
        fake_client = Mock(wraps=fake_juju_client())
        with patch('assess_constraints.prepare_constraint_test',
                   autospec=True, return_value={'cpu-power': '20'}
                   ) as prepare_mock:
            with self.assertRaises(JujuAssertionError):
                assess_cpu_power_constraints(fake_client, ['10', '30'])
        self.assertEqual(2, prepare_mock.call_count)

    def test_multiple_constraints(self):
        fake_client = Mock(wraps=fake_juju_client())
        with patch('assess_constraints.prepare_constraint_test',
                   autospec=True, side_effect=[
                       {'root-disk': '8G', 'cpu-power': '40'},
                       {'root-disk': '15G', 'cpu-power': '20'},
                       {'root-disk': '15G', 'cpu-power': '40'},
                       ]) as prepare_mock:
            assess_multiple_constraints(
                fake_client, 'test', root_disk='15G', cpu_power='40')
        self.assertEqual(3, prepare_mock.call_count)
        prepare_mock.assert_has_calls([
            call(fake_client, Constraints(cpu_power='40'), 'test-part0'),
            call(fake_client, Constraints(root_disk='15G'), 'test-part1'),
            call(fake_client, Constraints(root_disk='15G', cpu_power='40'),
                 'test-whole'),
            ])

    def test_multiple_constraints_not_met(self):
        fake_client = Mock(wraps=fake_juju_client())
        with patch('assess_constraints.prepare_constraint_test',
                   autospec=True, side_effect=[
                       {'root-disk': '8G', 'cpu-power': '40'},
                       {'root-disk': '15G', 'cpu-power': '20'},
                       {'root-disk': '15G', 'cpu-power': '30'},
                       ]):
            with self.assertRaisesRegexp(JujuAssertionError, 'Test Failed.*'):
                assess_multiple_constraints(
                    fake_client, 'test', root_disk='15G', cpu_power='40')

    def test_multiple_constraints_not_unique(self):
        fake_client = Mock(wraps=fake_juju_client())
        with patch('assess_constraints.prepare_constraint_test',
                   autospec=True, side_effect=[
                       {'root-disk': '15G', 'cpu-power': '40'},
                       {'root-disk': '15G', 'cpu-power': '20'},
                       {'root-disk': '15G', 'cpu-power': '40'},
                       ]):
            with self.assertRaisesRegexp(JujuAssertionError, 'Multiple.*'):
                assess_multiple_constraints(
                    fake_client, 'test', root_disk='15G', cpu_power='40')


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

    # Dictionaries showing plausable, but reduced and pre-loaded, output.
    SAMPLE_SHOW_MACHINE_OUTPUT = {
        'model': {'name': 'controller'},
        'machines': {'0': {'hardware': 'arch=amd64 cpu-cores=0 mem=0M'}},
        'applications': {}
        }

    SAMPLE_SHOW_MODEL_OUTPUT = {
        'model': 'UNUSED',
        'machines': {'0': 'UNUSED', '1': 'UNUSED'},
        'applications':
        {'wiki': {'units': {'wiki/0': {'machine': '0'}}},
         'mysql': {'units': {'mysql/0': {'machine': '1'}}},
         }}

    def test_machine_hardware(self):
        """Check the machine_hardware data translation."""
        output_mock = Mock(return_value=self.SAMPLE_SHOW_MACHINE_OUTPUT)
        fake_client = Mock(show_machine=output_mock)
        data = machine_hardware(fake_client, '0')
        output_mock.assert_called_once_with('0')
        self.assertEqual({'arch': 'amd64', 'cpu-cores': '0', 'mem': '0M'},
                         data)

    def test_application_machines(self):
        status = Status(self.SAMPLE_SHOW_MODEL_OUTPUT, '')
        output_mock = Mock(return_value=status)
        fake_client = Mock(get_status=output_mock)
        data = application_machines(fake_client, 'wiki')
        output_mock.assert_called_once_with()
        self.assertEquals(['0'], data)

    def test_application_hardware(self):
        fake_client = fake_juju_client()
        with patch('assess_constraints.application_machines',
                   autospec=True, return_value=['7']) as am_mock:
            with patch('assess_constraints.machine_hardware',
                       autospec=True, return_value='test') as mh_mock:
                data = application_hardware(fake_client, 'fakejob')
        self.assertEqual('test', data)
        am_mock.assert_called_once_with(fake_client, 'fakejob')
        mh_mock.assert_called_once_with(fake_client, '7')
