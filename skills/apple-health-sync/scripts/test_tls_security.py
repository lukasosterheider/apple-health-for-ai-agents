#!/usr/bin/env python3

from __future__ import annotations

import ssl
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest import mock

import certifi

import tls_security


class FakeNativeContext:
    def __init__(self, protocol: int) -> None:
        self.protocol = protocol
        self.check_hostname = False
        self.verify_mode = None
        self.minimum_version = None
        self.loaded_ca_file = ""

    def load_verify_locations(self, cafile: str) -> None:
        self.loaded_ca_file = cafile

    def cert_store_stats(self):
        raise NotImplementedError


class TLSConfigurationTests(unittest.TestCase):
    def test_native_trust_is_combined_with_bundled_fallback_and_kept_strict(self) -> None:
        fake_truststore = SimpleNamespace(SSLContext=FakeNativeContext)
        ca_file = Path(certifi.where())

        with mock.patch.object(tls_security, "truststore", fake_truststore):
            configured = tls_security.create_verified_tls_context(
                environ={},
                bundled_ca_override=ca_file,
            )

        self.assertEqual(configured.trust_source, "native+bundled")
        self.assertTrue(configured.bundled_ca_loaded)
        self.assertEqual(configured.context.loaded_ca_file, str(ca_file))
        self.assertTrue(configured.context.check_hostname)
        self.assertEqual(configured.context.verify_mode, ssl.CERT_REQUIRED)
        self.assertEqual(configured.context.minimum_version, ssl.TLSVersion.TLSv1_2)

    def test_explicit_ca_file_is_honored_without_loading_implicit_trust(self) -> None:
        configured = tls_security.create_verified_tls_context(
            environ={"SSL_CERT_FILE": certifi.where()},
        )

        self.assertEqual(configured.trust_source, "explicit")
        self.assertTrue(configured.explicit_ca_file)
        self.assertFalse(configured.bundled_ca_loaded)
        self.assertTrue(configured.context.check_hostname)
        self.assertEqual(configured.context.verify_mode, ssl.CERT_REQUIRED)

    def test_invalid_explicit_ca_file_fails_instead_of_silently_falling_back(self) -> None:
        with self.assertRaisesRegex(tls_security.TLSConfigurationError, "SSL_CERT_FILE"):
            tls_security.create_verified_tls_context(
                environ={"SSL_CERT_FILE": "/definitely/missing/healthsync-ca.pem"},
            )

    def test_invalid_explicit_ca_directory_fails_instead_of_silently_falling_back(self) -> None:
        with tempfile.TemporaryDirectory(prefix="healthsync-ca-test-") as temporary:
            file_instead_of_directory = Path(temporary) / "not-a-directory"
            file_instead_of_directory.write_text("not a CA directory", encoding="utf-8")
            with self.assertRaisesRegex(tls_security.TLSConfigurationError, "SSL_CERT_DIR"):
                tls_security.create_verified_tls_context(
                    environ={"SSL_CERT_DIR": str(file_instead_of_directory)},
                )

    def test_bundled_ca_remains_available_when_native_provider_cannot_initialize(self) -> None:
        failing_truststore = SimpleNamespace(SSLContext=mock.Mock(side_effect=OSError("unavailable")))

        with mock.patch.object(tls_security, "truststore", failing_truststore):
            configured = tls_security.create_verified_tls_context(
                environ={},
                bundled_ca_override=Path(certifi.where()),
            )

        self.assertEqual(configured.trust_source, "bundled")
        self.assertTrue(configured.bundled_ca_loaded)
        self.assertGreater(configured.ca_certificates or 0, 0)
        self.assertTrue(configured.context.check_hostname)
        self.assertEqual(configured.context.verify_mode, ssl.CERT_REQUIRED)

    def test_implementation_contains_no_tls_verification_bypass(self) -> None:
        source = Path(tls_security.__file__).read_text(encoding="utf-8")

        self.assertNotIn("_create_unverified_context", source)
        self.assertNotIn("CERT_NONE", source)
        self.assertNotIn("check_hostname = False", source)


if __name__ == "__main__":
    unittest.main()
