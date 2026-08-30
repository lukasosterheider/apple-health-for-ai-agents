#!/usr/bin/env python3
"""Strict TLS trust configuration for Health Sync relay requests."""

from __future__ import annotations

import os
import ssl
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping, Optional

try:
    import certifi
except ImportError:  # pragma: no cover - exercised by packaged fallback tests
    certifi = None

try:
    import truststore
except ImportError:  # pragma: no cover - exercised by packaged fallback tests
    truststore = None


TLS_MINIMUM_VERSION = ssl.TLSVersion.TLSv1_2


class TLSConfigurationError(RuntimeError):
    """Raised when strict certificate verification cannot be configured."""


@dataclass(frozen=True)
class VerifiedTLSContext:
    context: Any
    trust_source: str
    bundled_ca_loaded: bool
    explicit_ca_file: bool
    explicit_ca_dir: bool
    ca_certificates: Optional[int]

    def public_diagnostics(self) -> dict[str, Any]:
        return {
            "verification": "required",
            "hostname_verification": True,
            "minimum_tls": "1.2",
            "trust_source": self.trust_source,
            "bundled_ca_loaded": self.bundled_ca_loaded,
            "explicit_ca_file": self.explicit_ca_file,
            "explicit_ca_dir": self.explicit_ca_dir,
            "ca_certificates": self.ca_certificates,
        }


def bundled_resource_root() -> Path:
    pyinstaller_root = getattr(sys, "_MEIPASS", None)
    if pyinstaller_root:
        return Path(pyinstaller_root)
    return Path(__file__).resolve().parent.parent


def resolve_bundled_ca_file(override: Optional[Path] = None) -> Path:
    if override is not None:
        candidate = override
    else:
        packaged_candidate = bundled_resource_root() / "references" / "cacert.pem"
        if packaged_candidate.is_file():
            candidate = packaged_candidate
        elif certifi is not None:
            candidate = Path(certifi.where())
        else:
            raise TLSConfigurationError(
                "The bundled CA certificate file is missing. Reinstall the Health Sync runtime."
            )
    if not candidate.is_file():
        raise TLSConfigurationError(
            "The bundled CA certificate file is missing. Reinstall the Health Sync runtime."
        )
    return candidate


def _explicit_path(raw_value: str, label: str, expect_directory: bool) -> Optional[Path]:
    if not raw_value.strip():
        return None
    path = Path(raw_value).expanduser()
    path_is_valid = path.is_dir() if expect_directory else path.is_file()
    if not path_is_valid:
        expected = "directory" if expect_directory else "file"
        raise TLSConfigurationError(f"{label} does not point to a readable CA {expected}: {path}")
    return path


def _certificate_count(context: Any) -> Optional[int]:
    try:
        stats = context.cert_store_stats()
    except (AttributeError, NotImplementedError, ssl.SSLError):
        return None
    count = stats.get("x509_ca")
    return int(count) if isinstance(count, int) else None


def _enforce_strict_verification(context: Any) -> None:
    context.check_hostname = True
    context.verify_mode = ssl.CERT_REQUIRED
    context.minimum_version = TLS_MINIMUM_VERSION


def create_verified_tls_context(
    environ: Optional[Mapping[str, str]] = None,
    bundled_ca_override: Optional[Path] = None,
) -> VerifiedTLSContext:
    environment = os.environ if environ is None else environ
    explicit_ca_file = _explicit_path(
        environment.get("SSL_CERT_FILE", ""), "SSL_CERT_FILE", expect_directory=False
    )
    explicit_ca_dir = _explicit_path(
        environment.get("SSL_CERT_DIR", ""), "SSL_CERT_DIR", expect_directory=True
    )

    if explicit_ca_file is not None or explicit_ca_dir is not None:
        try:
            context = ssl.create_default_context(
                purpose=ssl.Purpose.SERVER_AUTH,
                cafile=str(explicit_ca_file) if explicit_ca_file else None,
                capath=str(explicit_ca_dir) if explicit_ca_dir else None,
            )
        except (OSError, ssl.SSLError) as error:
            raise TLSConfigurationError(
                f"Cannot load the explicitly configured CA trust: {error}"
            ) from error
        _enforce_strict_verification(context)
        return VerifiedTLSContext(
            context=context,
            trust_source="explicit",
            bundled_ca_loaded=False,
            explicit_ca_file=explicit_ca_file is not None,
            explicit_ca_dir=explicit_ca_dir is not None,
            ca_certificates=_certificate_count(context),
        )

    bundled_ca_file = resolve_bundled_ca_file(bundled_ca_override)
    context = None
    trust_source = "bundled"
    if truststore is not None:
        try:
            context = truststore.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
            context.load_verify_locations(cafile=str(bundled_ca_file))
            trust_source = "native+bundled"
        except (ImportError, NotImplementedError, OSError, RuntimeError, ValueError, ssl.SSLError):
            context = None

    if context is None:
        try:
            context = ssl.create_default_context(
                purpose=ssl.Purpose.SERVER_AUTH,
                cafile=str(bundled_ca_file),
            )
        except (OSError, ssl.SSLError) as error:
            raise TLSConfigurationError(f"Cannot load the bundled CA trust: {error}") from error

    _enforce_strict_verification(context)
    return VerifiedTLSContext(
        context=context,
        trust_source=trust_source,
        bundled_ca_loaded=True,
        explicit_ca_file=False,
        explicit_ca_dir=False,
        ca_certificates=_certificate_count(context),
    )
