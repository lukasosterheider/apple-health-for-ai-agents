#!/usr/bin/env python3
"""Require a complete published release before stable marketplace activation."""

from __future__ import annotations

import argparse
import json
import os
import urllib.error
import urllib.request
from pathlib import Path
from typing import Any

from build_distributions import REPOSITORY, VERSION


def required_asset_names(version: str) -> set[str]:
    runtimes = {
        f"apple-health-sync-runtime-{version}-darwin-arm64.tar.gz",
        f"apple-health-sync-runtime-{version}-darwin-x64.tar.gz",
        f"apple-health-sync-runtime-{version}-linux-arm64.tar.gz",
        f"apple-health-sync-runtime-{version}-linux-x64.tar.gz",
        f"apple-health-sync-runtime-{version}-windows-x64.zip",
    }
    return {
        f"apple-health-sync-agent-plugin-{version}.zip",
        f"apple-health-sync-codex-marketplace-{version}.zip",
        "apple-health-sync-claude-marketplace.json",
        f"apple-health-sync-agent-plugin-{version}.tgz",
        "apple-health-sync-agent-skill.zip",
        "apple-health-sync-agent-skill.zip.sha256",
        f"apple-health-sync-runtime-manifest-{version}.json",
        "release-provenance.json",
        "SHA256SUMS",
        *runtimes,
    }


def validate_release(payload: dict[str, Any], version: str) -> None:
    expected_tag = f"plugin-v{version}"
    if payload.get("tag_name") != expected_tag:
        raise RuntimeError(f"Release tag must be {expected_tag}")
    if payload.get("draft") is not False or payload.get("prerelease") is not False:
        raise RuntimeError(f"Release {expected_tag} must be a published stable release")
    available = {
        asset.get("name")
        for asset in payload.get("assets", [])
        if isinstance(asset, dict) and isinstance(asset.get("name"), str)
    }
    missing = sorted(required_asset_names(version) - available)
    if missing:
        raise RuntimeError("Release is missing required assets: " + ", ".join(missing))


def fetch_release(repository: str, version: str, token: str = "") -> dict[str, Any]:
    tag = f"plugin-v{version}"
    request = urllib.request.Request(
        f"https://api.github.com/repos/{repository}/releases/tags/{tag}",
        headers={
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
            **({"Authorization": f"Bearer {token}"} if token else {}),
        },
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            payload = json.load(response)
    except urllib.error.HTTPError as error:
        if error.code == 404:
            raise RuntimeError(f"Release {tag} does not exist") from error
        raise
    if not isinstance(payload, dict):
        raise RuntimeError("GitHub returned an invalid release response")
    return payload


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release-json", type=Path, help="Validate a saved API response")
    arguments = parser.parse_args()

    if arguments.release_json:
        payload = json.loads(arguments.release_json.read_text(encoding="utf-8"))
    else:
        payload = fetch_release(REPOSITORY, VERSION, os.environ.get("GITHUB_TOKEN", ""))
    validate_release(payload, VERSION)
    print(f"Published release plugin-v{VERSION} contains every required asset.")


if __name__ == "__main__":
    main()
