# Copyright 2012 Joyent, Inc.  All rights reserved.

"""A Python client/CLI/shell/SDK for Joyent Manta."""

from .version import __version__
from .client import MantaClient
from .auth import PrivateKeySigner, SSHAgentSigner, CLISigner
from .errors import *
