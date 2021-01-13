import os
from textwrap import dedent
from OpenSSL import crypto


def create_certificate(target_dir, ip_address):
    """Generate a cert and key file incl. IP SAN for `ip_address`

    Creates a cert.pem and key.pem file signed with a known ca cert.
    The generated cert will contain a IP SAN (subject alternative name) that
    includes the ip address of the server. This is required for log-forwarding.

    :return: tuple containing generated cert, key filepath pair

    """
    ip_address = 'IP:{}'.format(ip_address)

    key = crypto.PKey()
    key.generate_key(crypto.TYPE_RSA, 2048)

    csr_contents = generate_csr(target_dir, key, ip_address)
    req = crypto.load_certificate_request(crypto.FILETYPE_PEM, csr_contents)

    ca_cert = crypto.load_certificate(
        crypto.FILETYPE_PEM, ca_pem_contents)
    ca_key = crypto.load_privatekey(
        crypto.FILETYPE_PEM, ca_key_pem_contents)

    cert = crypto.X509()
    cert.set_version(0x2)
    cert.set_subject(req.get_subject())
    cert.set_serial_number(1)
    cert.gmtime_adj_notBefore(0)
    cert.gmtime_adj_notAfter(24 * 60 * 60)
    cert.set_issuer(ca_cert.get_subject())
    cert.set_pubkey(req.get_pubkey())
    cert.add_extensions([
        crypto.X509Extension('subjectAltName', False, ip_address),
        crypto.X509Extension(
            'extendedKeyUsage', False, 'clientAuth, serverAuth'),
        crypto.X509Extension(
            'keyUsage', True, 'keyEncipherment'),
    ])
    cert.sign(ca_key, "sha256")

    cert_filepath = os.path.join(target_dir, 'cert.pem')
    with open(cert_filepath, 'wt') as f:
        f.write(crypto.dump_certificate(crypto.FILETYPE_PEM, cert))

    key_filepath = os.path.join(target_dir, 'key.pem')
    with open(key_filepath, 'wt') as f:
        f.write(crypto.dump_privatekey(crypto.FILETYPE_PEM, key))

    return (cert_filepath, key_filepath)


def generate_csr(target_dir, key, ip_address):
    req = crypto.X509Req()
    req.set_version(0x2)
    req.get_subject().CN = "anyServer"
    # Add the IP SAN
    req.add_extensions([
        crypto.X509Extension("subjectAltName", False, ip_address)
    ])
    req.set_pubkey(key)
    req.sign(key, "sha256")

    return crypto.dump_certificate_request(crypto.FILETYPE_PEM, req)


ca_pem_contents = dedent("""\
    -----BEGIN CERTIFICATE-----
    MIIEFTCCAn2gAwIBAgIBBzANBgkqhkiG9w0BAQsFADAjMRIwEAYDVQQDEwlhbnlT
    ZXJ2ZXIxDTALBgNVBAoTBGp1anUwHhcNMTYwNzExMDQyOTM1WhcNMjYwNzA5MDQy
    OTM1WjAjMRIwEAYDVQQDEwlhbnlTZXJ2ZXIxDTALBgNVBAoTBGp1anUwggGiMA0G
    CSqGSIb3DQEBAQUAA4IBjwAwggGKAoIBgQCn6OxY33yAirABoE4UaZJBOnQORIzC
    125R71E2TG5gSHjHKA70L0C3dgyWhW9wcyhUbXBuz8Oep2J7kHvzuUPw2AWXI+Y2
    c0afWVqfj5kuyUpGhXsqylyf7NDPFs8hwGA6ZCFS3oUAvX8awsVucklxGeZNXZNK
    ZFilXKaX1Z3soORmKFZzVfDRqDuofZ2E0tmPh9C5gQ8qswjdBnTrj+0rCnvNekO0
    aND6AlkBHU+87pvcax0uUF6PYkXxPikKk1ftCQSII5oB5ksAtRpcZsYl5hT3U/t1
    DOA7c35RuIx7ogkcXP9jZ6J2tkmX+GMtUF29KEEnVCht32VDX+C3yS6lbfQB4oDt
    Yp3wXRY/LXTW7XTUrhoXB4nkYbw59gis5Cr7zDtUpiWFVYgy/kbxalljSM4N3w2i
    dtfxJHYjTfK98124qbCBb4A4ZNBJE2jy//lSIcIMXJv1LXQtTqR4rO1j6TBurohF
    NmUYpy3Zv7gn2CkfX6QfNFIj8elKT6dd+RUCAwEAAaNUMFIwDwYDVR0TAQH/BAUw
    AwEB/zAPBgNVHREECDAGhwQKwoylMA8GA1UdDwEB/wQFAwMHBAAwHQYDVR0OBBYE
    FP+v8GAqHiUCIygXbwWzbUhl/22DMA0GCSqGSIb3DQEBCwUAA4IBgQBVYKeT1O2M
    U3OPOy0IwqcA1/64rS1GlRmiw+papmjsy3aru03r8igahnbFd7wQawHaCScXbI/n
    OAPT4PDGXn6b71t4uHwWaM8wde159RO3G32N/VfhV6LPRUQunmAZh5QcJK6wWpYu
    B1f0dPkU+Q1AfX12oTOX/ld2/o7jaVswHoHoW6K2WQmwzlRQ953J+RJ7jXfrYDKl
    OAp3Hb69wAN4Ayc1s92iYUwV5q8UaHQoskHOLWJu964yFBHL8SLe6TLD+Jjv05Mc
    Ca7NKq/n25VTDNNaXl5MCNZ048m/GGHfktxxCddaF2grhC5HTUetwkq026PE0Wcq
    P+cDrIq6uTA25QqyBYistSa/7z2o0NBi56ySRqxlP2J2TPFZyOb+ZiA4EgYY5no5
    u2E+WuKZLVWl7eaQYOHgfYzFf3CvalSBwIjNynRwD/2Ebk7K29GPrIugb3V2+Vwh
    rltUXOHUkFGjEHIhr8zixfCxh5OzPJMnJwCZZRYzMO0/0Gw7ll9DmH0=
    -----END CERTIFICATE-----
    """)


ca_key_pem_contents = dedent("""\
    -----BEGIN RSA PRIVATE KEY-----
    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
    -----END RSA PRIVATE KEY-----
    """)
