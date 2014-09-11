from subprocess import CalledProcessError
from textwrap import dedent
from unittest import TestCase

from assess_recovery import (
    parse_new_state_server_from_error,
)


class AssessRecoveryTestCase(TestCase):

    def test_parse_new_state_server_from_error(self):
        output = dedent("""
            Waiting for address
            Attempting to connect to 10.0.0.202:22
            Attempting to connect to 1.2.3.4:22
            The fingerprint for the ECDSA key sent by the remote host is
            """)
        error = CalledProcessError(1, ['foo'], output)
        address = parse_new_state_server_from_error(error)
        self.assertEqual('1.2.3.4', address)
