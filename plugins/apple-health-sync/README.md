# Apple Health Sync plugin

This directory is the shared Codex and Claude Code plugin source. End users do not need Python,
pip, Node.js, or a compiler to run Apple Health Sync.

Supported release targets:

- macOS Apple silicon (`darwin-arm64`)
- macOS Intel (`darwin-x64`)
- Linux x86-64 (`linux-x64`)
- Linux ARM64 (`linux-arm64`)
- Windows x86-64 (`windows-x64`)

The source Python implementation remains under `skills/apple-health-sync/scripts/`. Build each
runtime on its target operating system with `helper-scripts/build_runtime_bundle.py`, then assemble
the release variants with `helper-scripts/build_plugin_distributions.py`.

The Codex Git marketplace contains only the manifests, skill, launchers, and a checksum-pinned
runtime index. On first use, its launcher downloads and atomically caches only the current
platform runtime. Claude Code and the manual offline ZIP continue to receive all native runtimes
under `runtime/<platform>/` so those packages remain fully self-contained.
