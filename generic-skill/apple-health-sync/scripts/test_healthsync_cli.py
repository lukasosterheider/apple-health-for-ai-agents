#!/usr/bin/env python3

from __future__ import annotations

import io
import json
import unittest
from contextlib import redirect_stdout
from unittest import mock

import healthsync_cli


class HealthSyncCliTests(unittest.TestCase):
    def test_self_test_is_local_and_reports_runtime_health(self) -> None:
        output = io.StringIO()

        with redirect_stdout(output):
            result = healthsync_cli.main(["self-test"])

        payload = json.loads(output.getvalue())
        self.assertEqual(result, 0)
        self.assertTrue(payload["ok"])
        self.assertEqual(payload["runtime_version"], healthsync_cli.RUNTIME_VERSION)
        self.assertEqual(payload["cryptography"], "ok")
        self.assertEqual(payload["tls"]["verification"], "required")
        self.assertTrue(payload["tls"]["hostname_verification"])
        self.assertTrue(payload["tls"]["bundled_ca_loaded"])

    def test_network_diagnostics_are_non_mutating_and_machine_readable(self) -> None:
        output = io.StringIO()
        diagnostics = {
            "ok": True,
            "endpoint": healthsync_cli.APP_CONFIG["supabase_qr_code_generator_url"],
            "method": "HEAD",
            "http_status": 405,
            "tls": {"verification": "required"},
        }

        with mock.patch.object(
            healthsync_cli,
            "diagnose_relay_https",
            return_value=diagnostics,
        ) as probe, redirect_stdout(output):
            result = healthsync_cli.main(["network-diagnostics", "--timeout-seconds", "7"])

        self.assertEqual(result, 0)
        self.assertEqual(json.loads(output.getvalue()), diagnostics)
        probe.assert_called_once_with(
            healthsync_cli.APP_CONFIG["supabase_qr_code_generator_url"],
            timeout=7,
        )

    def test_network_diagnostics_return_nonzero_without_raising_on_tls_failure(self) -> None:
        output = io.StringIO()
        with mock.patch.object(
            healthsync_cli,
            "diagnose_relay_https",
            side_effect=RuntimeError("certificate verify failed"),
        ), redirect_stdout(output):
            result = healthsync_cli.main(["network-diagnostics"])

        payload = json.loads(output.getvalue())
        self.assertEqual(result, 1)
        self.assertFalse(payload["ok"])
        self.assertIn("certificate verify failed", payload["error"])

    def test_summary_arguments_are_forwarded_without_mutating_global_argv(self) -> None:
        original_argv = list(healthsync_cli.sys.argv)
        observed_argv = []

        def command_main(arguments: list[str]) -> int:
            observed_argv.extend(healthsync_cli.sys.argv)
            self.assertEqual(arguments, ["--period", "daily"])
            return 0

        with mock.patch.object(healthsync_cli.create_data_summary, "main", side_effect=command_main) as command:
            result = healthsync_cli.main(["summary", "--period", "daily"])

        self.assertEqual(result, 0)
        command.assert_called_once_with(["--period", "daily"])
        self.assertEqual(observed_argv, ["healthsync summary", "--period", "daily"])
        self.assertEqual(healthsync_cli.sys.argv, original_argv)

    def test_script_commands_receive_scoped_sys_argv(self) -> None:
        original_argv = list(healthsync_cli.sys.argv)
        observed_argv = []

        def command_main() -> int:
            observed_argv.extend(healthsync_cli.sys.argv)
            return 0

        with mock.patch.object(healthsync_cli.onboarding, "main", side_effect=command_main) as command:
            result = healthsync_cli.main(["onboarding", "--protocol", "v5"])

        self.assertEqual(result, 0)
        command.assert_called_once_with()
        self.assertEqual(observed_argv, ["healthsync onboarding", "--protocol", "v5"])
        self.assertEqual(healthsync_cli.sys.argv, original_argv)


if __name__ == "__main__":
    unittest.main()
