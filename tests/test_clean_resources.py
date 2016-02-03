from argparse import Namespace
from mock import (
    MagicMock,
    patch,
)

from clean_resources import (
    parse_args,
    get_regions,
    main,
)
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
        args = Namespace(all_regions=False)
        env = MagicMock()
        env.config = {'region': 'foo'}
        regions = get_regions(args, env)
        self.assertEqual(regions, ['foo'])

    def test_get_regions_all_regions(self):
        args = Namespace(all_regions=True)
        expected_output = ['ap-southeast-1', 'ap-southeast-2', 'us-west-2',
                           'us-east-1', 'us-west-1', 'sa-east-1',
                           'ap-northeast-1', 'eu-west-1']
        self.assertEqual(get_regions(args, None), expected_output)

    def test_main_all_regions(self):
        args = Namespace(all_regions=True)
        self.asses_main(all_region=True,
                        call_count=len(get_regions(args, None)))

    def test_main_single_region(self):
        self.asses_main(all_region=False, call_count=1)

    def asses_main(self, all_region, call_count):
        mock_isg = patch(
            'clean_resources.AWSAccount.iter_security_groups',
            autospec=True, return_value={})
        mock_iisg = patch(
            'clean_resources.AWSAccount.iter_instance_security_groups',
            autospec=True, return_value={})
        mock_ddi = patch(
            'clean_resources.AWSAccount.delete_detached_interfaces',
            autospec=True, return_value={})
        mock_dsg = patch(
            'clean_resources.AWSAccount.destroy_security_groups',
            autospec=True, return_value={})
        mock_pa = patch(
            'clean_resources.parse_args', autospec=True,
            return_value=Namespace(
                env='foo', verbose=0, all_regions=all_region))
        with mock_pa as pa:
            with patch('clean_resources.SimpleEnvironment.from_config',
                       return_value=get_aws_env()) as cr_mock:
                with mock_isg as isg:
                    with mock_iisg as iisg:
                        with mock_ddi as ddi:
                            with mock_dsg as dsg:
                                main()
        self.assertEqual(dsg.call_count, call_count)
        self.assertEqual(ddi.call_count, call_count)
        self.assertEqual(iisg.call_count, call_count)
        self.assertEqual(isg.call_count, call_count)
        self.assertEqual(pa.call_count, 1)
        cr_mock.assert_called_once_with('foo')
