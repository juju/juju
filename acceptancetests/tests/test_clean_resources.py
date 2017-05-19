from argparse import Namespace
from mock import (
    call,
    patch,
)

from clean_resources import (
    clean,
    get_regions,
    main,
    parse_args,
)
import clean_resources
from tests import TestCase
from tests.test_substrate import get_aws_env

__metaclass__ = type


class CleanResources(TestCase):

    def test_parse_args_default(self):
        args = parse_args(['default-aws'])
        self.assertEqual(args, Namespace(all_regions=False, env='default-aws',
                                         verbose=0))

    def test_parse_args_all_regions(self):
        args = parse_args(['default-aws', '--verbose', '--all-regions'])
        self.assertEqual(args, Namespace(all_regions=True, env='default-aws',
                                         verbose=1))

    def test_get_regions(self):
        class FakeEnv:
            def get_region(self):
                return 'foo'
        args = Namespace(all_regions=False)
        env = FakeEnv()
        regions = get_regions(args, env)
        self.assertEqual(regions, ['foo'])

    def test_get_regions_all_regions(self):
        args = Namespace(all_regions=True)
        supported_regions = {'ap-southeast-1', 'ap-southeast-2',
                             'us-west-2', 'us-east-1', 'us-west-1',
                             'sa-east-1', 'ap-northeast-1', 'eu-west-1'}
        all_regions = set(get_regions(args, None))
        self.assertTrue(all_regions.issuperset(supported_regions))

    def test_clean_all_regions(self):
        args = Namespace(all_regions=True)
        self.asses_clean(all_region=True,
                         call_count=len(get_regions(args, None)))

    def test_clean_single_region(self):
        self.asses_clean(all_region=False, call_count=1)

    def asses_clean(self, all_region, call_count):
        args = Namespace(env='foo', verbose=0, all_regions=all_region)
        env = get_aws_env()
        with patch.object(clean_resources.AWSAccount,
                          'from_boot_config') as fbc_mock:
            with patch('clean_resources.SimpleEnvironment.from_config',
                       return_value=env) as cr_mock:
                clean(args)
        self.assertEqual(
            fbc_mock.call_count,
            call_count)
        regions = get_regions(args, env)
        calls = [call(cr_mock.return_value, region=r) for r in regions]
        self.assertEqual(fbc_mock.call_args_list, calls)
        ctx_mock = fbc_mock.return_value.__enter__.return_value
        self.assertEqual(ctx_mock.iter_security_groups.call_count, call_count)
        self.assertEqual(
            ctx_mock.iter_instance_security_groups.call_count, call_count)
        self.assertEqual(
            ctx_mock.delete_detached_interfaces.call_count, call_count)
        self.assertEqual(
            ctx_mock.destroy_security_groups.call_count, call_count)
        cr_mock.assert_called_once_with('foo')

    def test_main(self):
        args = Namespace(env='foo', verbose=0, all_regions=True)
        with patch('clean_resources.parse_args', autospec=True,
                   return_value=args) as pa_mock:
            with patch('clean_resources.clean', autospec=True) as cln_mock:
                main()
        pa_mock.assert_called_once_with()
        cln_mock.assert_called_once_with(args)
