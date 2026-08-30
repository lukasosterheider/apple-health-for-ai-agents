import json
import os
import secrets
import unittest
from pathlib import Path
from urllib import error, request
from unittest import mock

from config import (
    ALLOWED_RELAY_URLS,
    APP_CONFIG,
    RejectRelayRedirects,
    build_relay_opener,
    diagnose_relay_https,
    load_effective_config,
    validate_relay_url,
)
from tls_security import VerifiedTLSContext


class ConfigSecurityTests(unittest.TestCase):
    @staticmethod
    def fake_tls() -> VerifiedTLSContext:
        return VerifiedTLSContext(
            context=mock.Mock(),
            trust_source="native+bundled",
            bundled_ca_loaded=True,
            explicit_ca_file=False,
            explicit_ca_dir=False,
            ca_certificates=None,
        )

    def test_declared_relay_urls_are_allowed(self) -> None:
        for relay_url in ALLOWED_RELAY_URLS:
            self.assertEqual(validate_relay_url(relay_url), relay_url)

    def test_other_hosts_paths_and_queries_are_rejected(self) -> None:
        rejected_urls = (
            "https://example.com/functions/v1/get-data-v2",
            "http://snpiylxajnxpklpwdtdg.supabase.co/functions/v1/get-data-v2",
            "https://snpiylxajnxpklpwdtdg.supabase.co/functions/v1/unexpected",
            "https://snpiylxajnxpklpwdtdg.supabase.co/functions/v1/get-data-v2?redirect=1",
        )

        for relay_url in rejected_urls:
            with self.subTest(relay_url=relay_url):
                with self.assertRaisesRegex(RuntimeError, "undeclared relay URL"):
                    validate_relay_url(relay_url)

    def test_relay_redirects_are_rejected_before_following_target(self) -> None:
        relay_url = next(iter(ALLOWED_RELAY_URLS))
        relay_request = request.Request(relay_url)

        with self.assertRaisesRegex(error.HTTPError, "Relay redirects are not allowed") as caught:
            RejectRelayRedirects().redirect_request(
                relay_request,
                None,
                302,
                "Found",
                {},
                "https://example.com/collect",
            )
        caught.exception.close()

    def test_relay_opener_uses_the_verified_tls_context(self) -> None:
        tls = self.fake_tls()
        opener = mock.Mock()
        with mock.patch("config.create_verified_tls_context", return_value=tls), mock.patch(
            "config.request.HTTPSHandler"
        ) as https_handler, mock.patch("config.request.build_opener", return_value=opener):
            configured_opener, configured_tls = build_relay_opener()

        https_handler.assert_called_once_with(context=tls.context)
        self.assertIs(configured_opener, opener)
        self.assertIs(configured_tls, tls)

    def test_https_diagnostic_treats_an_http_error_as_a_completed_tls_handshake(self) -> None:
        relay_url = APP_CONFIG["supabase_qr_code_generator_url"]
        opener = mock.Mock()
        opener.open.side_effect = error.HTTPError(relay_url, 405, "Method Not Allowed", {}, None)
        with mock.patch("config.build_relay_opener", return_value=(opener, self.fake_tls())):
            diagnostics = diagnose_relay_https(relay_url)

        self.assertTrue(diagnostics["ok"])
        self.assertEqual(diagnostics["method"], "HEAD")
        self.assertEqual(diagnostics["http_status"], 405)
        self.assertEqual(diagnostics["tls"]["verification"], "required")

    def test_https_diagnostic_reports_transport_failures(self) -> None:
        relay_url = APP_CONFIG["supabase_qr_code_generator_url"]
        opener = mock.Mock()
        opener.open.side_effect = error.URLError("certificate verify failed")
        with mock.patch("config.build_relay_opener", return_value=(opener, self.fake_tls())):
            with self.assertRaisesRegex(RuntimeError, "Verified HTTPS probe failed"):
                diagnose_relay_https(relay_url)

    def test_user_config_cannot_override_app_owned_relay_url(self) -> None:
        runtime_dir = Path("/tmp") / f"ahs-config-test-{secrets.token_hex(8)}"
        config_dir = runtime_dir / "config"
        config_dir.mkdir(mode=0o700, parents=True)
        config_path = config_dir / "config.json"
        config_path.write_text(
            json.dumps(
                {
                    "user_id": "ahs_test",
                    "storage": "sqlite",
                    "supabase_get_data_url": "https://example.com/collect",
                }
            ),
            encoding="utf-8",
        )
        os.chmod(config_path, 0o644)
        try:
            _, effective_config = load_effective_config(runtime_dir)
            self.assertEqual(effective_config["supabase_get_data_url"], APP_CONFIG["supabase_get_data_url"])
            self.assertEqual(config_path.stat().st_mode & 0o777, 0o600)
        finally:
            config_path.unlink(missing_ok=True)
            config_dir.rmdir()
            runtime_dir.rmdir()


if __name__ == "__main__":
    unittest.main()
