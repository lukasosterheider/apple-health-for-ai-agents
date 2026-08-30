# Apple Health Sync plugin

This package integrates Apple Health Sync with Codex and Claude Code. End users do not need Python,
pip, Node.js, or a compiler to run Apple Health Sync.

Supported release targets:

- macOS Apple silicon (`darwin-arm64`)
- macOS Intel (`darwin-x64`)
- Linux x86-64 (`linux-x64`)
- Linux ARM64 (`linux-arm64`)
- Windows x86-64 (`windows-x64`)

The Python implementation is maintained in the public
[portable skill source](https://github.com/lukasosterheider/apple-health-for-ai-agents/tree/main/generic-skill/apple-health-sync).
Maintainer build commands run from the `Private` checkout, with `Public` beside it.
Build each runtime on its target operating system with
`helper-scripts/build_runtime_bundle.py`, then assemble release variants with
`helper-scripts/build_plugin_distributions.py`. Those helpers are private release
tooling and are not part of this public package. Update generated plugin files
through their private source templates; edit portable skills directly in Public.

The Codex Git marketplace contains only the manifests, skill, launchers, and a checksum-pinned
runtime index. On first use, its launcher downloads and atomically caches only the current
platform runtime. Claude Code and the manual offline ZIP continue to receive all native runtimes
under `runtime/<platform>/` so those packages remain fully self-contained.
