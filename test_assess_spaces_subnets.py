import assess_spaces_subnets as jss
from test_assess_container_networking import JujuMockTestCase


class TestSubnetsSpaces(JujuMockTestCase):
    def test_ipv4_to_int(self):
        self.assertEqual(
            jss.ipv4_to_int('1.2.3.4'),
            0x01020304)

        self.assertEqual(
            jss.ipv4_to_int('255.255.255.255'),
            0xFFFFFFFF)

    def test_ipv4_in_cidr(self):
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.1.1/32'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.1.0/24'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.0.0/16'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.0.0.0/8'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '0.0.0.0/0'))

        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.1.1/32'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.1.0/24'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.0.0/16'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.0.0.0/8'))

    def test_assess_spaces_subnets(self):
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.1.0.2'], '1')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '2')
        self.juju_mock.add_machine('1')
        self.juju_mock.add_machine('2')
        self.assertEqual(jss.assess_spaces_subnets(self.client), 2)

    def test_assess_spaces_subnets_fail(self):
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '1')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.1.0.2'], '2')
        self.juju_mock.add_machine('1')
        self.juju_mock.add_machine('2')
        self.assertRaisesRegexp(
            ValueError, 'Found ubuntu-public/0 in dmz, expected public',
            jss.assess_spaces_subnets, self.client)
