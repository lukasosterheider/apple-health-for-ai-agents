#!/usr/bin/env python3
"""Self-contained command-line entry point for Apple Health Sync."""

from __future__ import annotations

import argparse
import json
import sys
from types import ModuleType
from typing import Sequence

import create_data_summary
import fetch_health_data
import onboarding
import unlink_device
from config import APP_CONFIG, diagnose_relay_https, load_defaults_config
from cryptography.hazmat.primitives.asymmetric import ed25519
from tls_security import create_verified_tls_context


RUNTIME_VERSION = "1.1.1"
COMMANDS: dict[str, tuple[str, ModuleType, bool]] = {
    "onboarding": ("Initialize or rotate the encrypted local identity.", onboarding, False),
    "fetch": ("Fetch, decrypt, validate, and store Apple Health data.", fetch_health_data, False),
    "unlink": ("Unlink the currently paired iOS device.", unlink_device, False),
    "summary": ("Create a local daily, weekly, or monthly summary.", create_data_summary, True),
}


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="healthsync",
        description="Secure Apple Health Sync runtime for AI-agent plugins.",
    )
    parser.add_argument("--version", action="version", version=f"healthsync {RUNTIME_VERSION}")
    subparsers = parser.add_subparsers(dest="command", metavar="COMMAND")
    for name, (description, _, _) in COMMANDS.items():
        subparsers.add_parser(name, add_help=False, help=description)
    subparsers.add_parser(
        "self-test",
        add_help=False,
        help="Verify bundled resources, cryptography, and strict TLS trust without network access.",
    )
    network_diagnostics = subparsers.add_parser(
        "network-diagnostics",
        add_help=False,
        help="Test verified HTTPS access to the QR endpoint without changing identity or health data.",
    )
    network_diagnostics.add_argument("--timeout-seconds", type=int, default=10)
    return parser


def run_self_test() -> int:
    defaults = load_defaults_config()
    tls = create_verified_tls_context()
    private_key = ed25519.Ed25519PrivateKey.generate()
    message = b"healthsync-self-test"
    private_key.public_key().verify(private_key.sign(message), message)
    print(
        json.dumps(
            {
                "ok": True,
                "runtime_version": RUNTIME_VERSION,
                "default_storage": defaults["storage"],
                "cryptography": "ok",
                "tls": tls.public_diagnostics(),
            },
            separators=(",", ":"),
        )
    )
    return 0


def run_network_diagnostics(timeout_seconds: int) -> int:
    if timeout_seconds < 1 or timeout_seconds > 60:
        raise ValueError("--timeout-seconds must be between 1 and 60")
    try:
        diagnostics = diagnose_relay_https(
            str(APP_CONFIG["supabase_qr_code_generator_url"]),
            timeout=timeout_seconds,
        )
    except Exception as error:
        print(
            json.dumps(
                {
                    "ok": False,
                    "endpoint": str(APP_CONFIG["supabase_qr_code_generator_url"]),
                    "method": "HEAD",
                    "error": str(error),
                },
                separators=(",", ":"),
            )
        )
        return 1
    print(json.dumps(diagnostics, separators=(",", ":")))
    return 0


def main(argv: Sequence[str] | None = None) -> int:
    arguments = list(argv if argv is not None else sys.argv[1:])
    parser = build_parser()
    parsed, command_arguments = parser.parse_known_args(arguments)
    if not parsed.command:
        parser.print_help()
        return 2
    if parsed.command == "self-test":
        if command_arguments:
            parser.error("self-test does not accept additional arguments")
        return run_self_test()
    if parsed.command == "network-diagnostics":
        if command_arguments:
            parser.error("network-diagnostics received unsupported arguments")
        return run_network_diagnostics(parsed.timeout_seconds)

    _, command_module, accepts_arguments = COMMANDS[parsed.command]
    command_main = command_module.main
    previous_argv = sys.argv
    try:
        sys.argv = [f"healthsync {parsed.command}", *command_arguments]
        if accepts_arguments:
            return command_main(command_arguments)
        return command_main()
    finally:
        sys.argv = previous_argv


if __name__ == "__main__":
    raise SystemExit(main())
