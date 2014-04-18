# Copyright 2012 Joyent, Inc.  All rights reserved.

"""Manta client auth."""

import sys
import os
from os.path import exists, expanduser, join, dirname, abspath
import logging
import base64
import hashlib
from getpass import getpass
import re
import struct
from glob import glob

try:
    from Crypto.PublicKey import RSA
    from Crypto.Signature import PKCS1_v1_5
    from Crypto.Hash import SHA256, SHA, SHA512
except ImportError:
    sys.stderr.write(
        "* * *\n"
        "See <https://github.com/joyent/python-manta#1-pycrypto-dependency>\n"
        "for help installing PyCrypto (the Python 'Crypto' package)\n"
        "* * *\n")
    raise
import paramiko

from manta.errors import MantaError



#---- globals

log = logging.getLogger('manta.auth')

FINGERPRINT_RE = re.compile(r'^([a-f0-9]{2}:){15}[a-f0-9]{2}$');



#---- internal support stuff

def fingerprint_from_ssh_pub_key(data):
    """Calculate the fingerprint of SSH public key data.

    >>> data = "ssh-rsa AAAAB3NzaC1y...4IEAA1Z4wIWCuk8F9Tzw== my key comment"
    >>> fingerprint_from_ssh_pub_key(data)
    '54:c7:4c:93:cf:ff:e3:32:68:bc:89:6e:5e:22:b5:9c'

    Adapted from <http://stackoverflow.com/questions/6682815/>
    and imgapi.js#fingerprintFromSshpubkey.
    """
    data = data.strip()

    # Let's accept either:
    # - just the base64 encoded data part, e.g.
    #   'AAAAB3NzaC1yc2EAAAABIwAA...2l24uq9Lfw=='
    # - the full ssh pub key file content, e.g.:
    #   'ssh-rsa AAAAB3NzaC1yc2EAAAABIwAA...2l24uq9Lfw== my comment'
    if (re.search(r'^ssh-(?:rsa|dss) ', data)):
        data = data.split(None, 1)[1]

    key = base64.b64decode(data)
    fp_plain = hashlib.md5(key).hexdigest()
    return ':'.join(a+b for a,b in zip(fp_plain[::2], fp_plain[1::2]))

def fingerprint_from_raw_ssh_pub_key(key):
    """Encode a raw SSH key (string of bytes, as from
    `str(paramiko.AgentKey)`) to a fingerprint in the typical
    '54:c7:4c:93:cf:ff:e3:32:68:bc:89:6e:5e:22:b5:9c' form.
    """
    fp_plain = hashlib.md5(key).hexdigest()
    return ':'.join(a+b for a,b in zip(fp_plain[::2], fp_plain[1::2]))


def load_ssh_key(key_id, skip_priv_key=False):
    """
    Load a local ssh private key (in PEM format). PEM format is the OpenSSH
    default format for private keys.

    See similar code in imgapi.js#loadSSHKey.

    @param key_id {str} An ssh public key fingerprint or ssh private key path.
    @param skip_priv_key {boolean} Optional. Default false. If true, then this
        will skip loading the private key file and `priv_key` will be `None`
        in the retval.
    @returns {dict} with these keys:
        - pub_key_path
        - fingerprint
        - priv_key_path
        - priv_key
    """
    priv_key = None

    # If `key_id` is already a private key path, then easy.
    if not FINGERPRINT_RE.match(key_id):
        if not skip_priv_key:
            f = open(key_id)
            try:
                priv_key = f.read()
            finally:
                f.close()
        pub_key_path = key_id + '.pub'
        f = open(pub_key_path)
        try:
            pub_key = f.read()
        finally:
            f.close()
        fingerprint = fingerprint_from_ssh_pub_key(pub_key)
        return dict(
            pub_key_path=pub_key_path,
            fingerprint=fingerprint,
            priv_key_path=key_id,
            priv_key=priv_key)

    # Else, look at all pub/priv keys in "~/.ssh" for a matching fingerprint.
    fingerprint = key_id
    pub_key_glob = expanduser('~/.ssh/*.pub')
    for pub_key_path in glob(pub_key_glob):
        f = open(pub_key_path)
        try:
            pub_key = f.read()
        finally:
            f.close()
        if fingerprint_from_ssh_pub_key(pub_key) == fingerprint:
            break
    else:
        raise MantaError(
            "no '~/.ssh/*.pub' key found with fingerprint '%s'"
            % fingerprint)
    priv_key_path = os.path.splitext(pub_key_path)[0]
    if not skip_priv_key:
        f = open(priv_key_path)
        try:
            priv_key = f.read()
        finally:
            f.close()
    return dict(
        pub_key_path=pub_key_path,
        fingerprint=fingerprint,
        priv_key_path=priv_key_path,
        priv_key=priv_key)


def unpack_agent_response(d):
    parts = []
    while d:
        length = struct.unpack('>I', d[:4])[0]
        bits = d[4:length+4]
        parts.append(bits)
        d = d[length+4:]
    return parts

def signature_from_agent_sign_response(d):
    """h/t <https://github.com/atl/py-http-signature/blob/master/http_signature/sign.py>"""
    return unpack_agent_response(d)[1]

def ssh_key_info_from_key_data(key_id, priv_key=None):
    """Get/load SSH key info necessary for signing.

    @param key_id {str} Either a private ssh key fingerprint, e.g.
        'b3:f0:a1:6c:18:3b:42:63:fd:6e:57:42:74:17:d4:bc', or the path to
        an ssh private key file (like ssh's IdentityFile config option).
    @param priv_key {str} Optional. SSH private key file data (PEM format).
    @return {dict} with these keys:
        - type: "agent"
        - signer: Crypto signer class (a PKCS#1 v1.5 signer for RSA keys)
        - fingerprint: key fingerprint
        - algorithm: 'rsa-sha256'  DSA not current supported. Hash algorithm
          selection is not exposed.
        - ... some others added by `load_ssh_key()`
    """
    if FINGERPRINT_RE.match(key_id) and priv_key:
        key_info = {
            "fingerprint": key_id,
            "priv_key": priv_key
        }
    else:
        # Otherwise, we attempt to load necessary details from ~/.ssh.
        key_info = load_ssh_key(key_id)

    # Load an RSA key signer.
    key = None
    try:
        key = RSA.importKey(key_info["priv_key"])
    except ValueError:
        if "priv_key_path" in key_info:
            prompt = "Passphrase [%s]: " % key_info["priv_key_path"]
        else:
            prompt = "Passphrase: "
        for i in range(3):
            passphrase = getpass(prompt)
            if not passphrase:
                break
            try:
                key = RSA.importKey(key_info["priv_key"], passphrase)
            except ValueError:
                continue
            else:
                break
        if not key:
            details = ""
            if "priv_key_path" in key_info:
                details = " (%s)" % key_info["priv_key_path"]
            raise MantaError("could not import key" + details)
    key_info["signer"] = PKCS1_v1_5.new(key)

    key_info["type"] = "ssh_key"
    key_info["algorithm"] = "rsa-sha256"
    return key_info


def agent_key_info_from_key_id(key_id):
    """Find a matching key in the ssh-agent.

    @param key_id {str} Either a private ssh key fingerprint, e.g.
        'b3:f0:a1:6c:18:3b:42:63:fd:6e:57:42:74:17:d4:bc', or the path to
        an ssh private key file (like ssh's IdentityFile config option).
    @return {dict} with these keys:
        - type: "agent"
        - agent_key: paramiko AgentKey
        - fingerprint: key fingerprint
        - algorithm: "rsa-sha1"  Currently don't support DSA agent signing.
    """
    # Need the fingerprint of the key we're using for signing. If it
    # is a path to a priv key, then we need to load it.
    if not FINGERPRINT_RE.match(key_id):
        ssh_key = load_ssh_key(key_id, True)
        fingerprint = ssh_key["fingerprint"]
    else:
        fingerprint = key_id

    # Look for a matching fingerprint in the ssh-agent keys.
    import paramiko
    keys = paramiko.Agent().get_keys()
    for key in keys:
        if fingerprint_from_raw_ssh_pub_key(str(key)) == fingerprint:
            break
    else:
        raise MantaError(
            'no ssh-agent key with fingerprint "%s"' % fingerprint)

    # TODO:XXX DSA support possible with paramiko?
    algorithm = 'rsa-sha1'

    return {
        "type": "agent",
        "agent_key": key,
        "fingerprint": fingerprint,
        "algorithm": algorithm
    }



#---- exports

class Signer(object):
    """A virtual base class for python-manta request signing."""
    def sign(self, s):
        """Sign the given string.

        @param s {str} The string to be signed.
        @returns (algorithm, key-fingerprint, signature) {3-tuple}
            For example: `("rsa-sha256", "b3:f0:...:bc",
            "OXKzi5+h1aR9dVWHOu647x+ijhk...6w==")`.
        """
        raise NotImplementedError("this is a virtual base class")

class PrivateKeySigner(Signer):
    """Sign Manta requests with the given ssh private key.

    @param key_id {str} Either a private ssh key fingerprint, e.g.
        'b3:f0:a1:6c:18:3b:42:63:fd:6e:57:42:74:17:d4:bc', or the path to
        an ssh private key file (like ssh's IdentityFile config option).
    @param priv_key {str} Optional. SSH private key file data (PEM format).

    If a *fingerprint* is provided for `key_id` *and* `priv_key` is specified,
    then this is all the data required. Otherwise, this class will attempt
    to load required key data (both public and private key files) from
    keys in "~/.ssh/".
    """
    def __init__(self, key_id, priv_key=None):
        self.key_id = key_id
        self.priv_key = priv_key

    _key_info_cache = None
    def _get_key_info(self):
        """Get key info appropriate for signing."""
        if self._key_info_cache is None:
            self._key_info_cache = ssh_key_info_from_key_data(
                self.key_id, self.priv_key)
        return self._key_info_cache

    def sign(self, s):
        assert isinstance(s, str)   # for now, not unicode. Python 3?

        key_info = self._get_key_info()

        assert key_info["type"] == "ssh_key"
        hash_algo = key_info["algorithm"].split('-')[1]
        hash_class = {
            "sha1": SHA,
            "sha256": SHA256,
            "sha512": SHA512
        }[hash_algo]
        hasher = hash_class.new()
        hasher.update(s)
        signed_raw = key_info["signer"].sign(hasher)
        signed = base64.b64encode(signed_raw)

        return (key_info["algorithm"], key_info["fingerprint"], signed)

class SSHAgentSigner(Signer):
    """Sign Manta requests using an ssh-agent.

    @param key_id {str} Either a private ssh key fingerprint, e.g.
        'b3:f0:a1:6c:18:3b:42:63:fd:6e:57:42:74:17:d4:bc', or the path to
        an ssh private key file (like ssh's IdentityFile config option).
    """
    def __init__(self, key_id):
        self.key_id = key_id

    _key_info_cache = None
    def _get_key_info(self):
        """Get key info appropriate for signing."""
        if self._key_info_cache is None:
            self._key_info_cache = agent_key_info_from_key_id(self.key_id)
        return self._key_info_cache

    def sign(self, s):
        assert isinstance(s, str)   # for now, not unicode. Python 3?

        key_info = self._get_key_info()
        assert key_info["type"] == "agent"
        response = key_info["agent_key"].sign_ssh_data(None, s)
        signed_raw = signature_from_agent_sign_response(response)
        signed = base64.b64encode(signed_raw)

        return (key_info["algorithm"], key_info["fingerprint"], signed)

class CLISigner(Signer):
    """Sign Manta requests using the SSH agent (if available and has the
    required key) or loading keys from "~/.ssh/*".
    """
    def __init__(self, key_id):
        self.key_id = key_id

    _key_info_cache = None
    def _get_key_info(self):
        """Get key info appropriate for signing: either from the ssh agent
        or from a private key.
        """
        if self._key_info_cache is not None:
            return self._key_info_cache

        errors = []

        # First try the agent.
        try:
            key_info = agent_key_info_from_key_id(self.key_id)
        except MantaError:
            _, ex, _ = sys.exc_info()
            errors.append(ex)
        else:
            self._key_info_cache = key_info
            return self._key_info_cache

        # Try loading from "~/.ssh/*".
        try:
            key_info = ssh_key_info_from_key_data(self.key_id)
        except MantaError:
            _, ex, _ = sys.exc_info()
            errors.append(ex)
        else:
            self._key_info_cache = key_info
            return self._key_info_cache

        raise MantaError("could not find key info for signing: %s"
            % "; ".join(map(unicode, errors)))

    def sign(self, sigstr):
        assert isinstance(sigstr, str)   # for now, not unicode. Python 3?

        key_info = self._get_key_info()
        log.debug("sign %r with %s key (algo %s, fp %s)", sigstr,
            key_info["type"], key_info["algorithm"], key_info["fingerprint"])

        if key_info["type"] == "agent":
            response = key_info["agent_key"].sign_ssh_data(None, sigstr)
            signed_raw = signature_from_agent_sign_response(response)
            signed = base64.b64encode(signed_raw)
        elif key_info["type"] == "ssh_key":
            hash_algo = key_info["algorithm"].split('-')[1]
            hash_class = {
                "sha1": SHA,
                "sha256": SHA256,
                "sha512": SHA512
            }[hash_algo]
            hasher = hash_class.new()
            hasher.update(sigstr)
            signed_raw = key_info["signer"].sign(hasher)
            signed = base64.b64encode(signed_raw)
        else:
            raise MantaError("internal error: unknown key type: %r"
                % key_info["type"])

        return (key_info["algorithm"], key_info["fingerprint"], signed)
