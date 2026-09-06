# Apple Health Sync for Codex

## Install

With the Codex CLI installed, run:

```bash
codex plugin marketplace add https://github.com/lukasosterheider/apple-health-for-ai-agents.git
codex plugin add apple-health-sync@healthsync
```

Restart Codex and start a new task. On first use, the plugin downloads the matching
Health Sync executable and verifies its checksum. No Python installation is needed.

## Start onboarding and Connect your iPhone

Send this in your new task:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

Open [Health Sync on your iPhone](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose **QR Code Setup**, scan the code, grant Apple Health permissions and complete
your first sync. Then ask Codex: `Fetch my health data and summarize this week.`

[Illustrated guide](https://gethealthsync.app/#install-codex)
· [CLI commands](../cli/README.md#commands)
