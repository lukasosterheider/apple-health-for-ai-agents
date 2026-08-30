# Apple Health Sync for Codex

Install the native plugin using the commands from the
[website installation guide](https://gethealthsync.app/#install-codex).
Python and pip are not required on the user's computer.

## Install

With the Codex CLI available in your terminal, run:

```bash
codex plugin marketplace add https://github.com/lukasosterheider/apple-health-for-ai-agents.git
codex plugin add apple-health-sync@healthsync
```

Restart Codex and start a new task. On first use, the plugin downloads only the
runtime for your operating system and verifies the package and executable checksums.

## Start onboarding and Connect your iPhone

Send this in the new task:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

Approve initialization when prompted. Open
[Health Sync on your iPhone](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose QR Code Setup, scan the code, select Apple Health permissions, and complete
the first sync. Then ask Codex: `What Apple Health data can I access?`

## Package layout

[`apple-health-sync/`](apple-health-sync) contains the generated plugin, launchers,
and runtime manifests. The marketplace entry stays at
[`/.agents/plugins/marketplace.json`](../.agents/plugins/marketplace.json) so the
repository URL remains installable. Its local source points into this directory.

Existing release tags keep their original package paths. Native runtimes and
offline bundles remain available in
[GitHub Releases](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases).
The editable Python source is in
[`generic-skill`](../generic-skill); generated plugin changes belong in the
maintainer's private templates.
