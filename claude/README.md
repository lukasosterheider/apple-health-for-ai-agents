# Apple Health Sync for Claude

Claude Code uses the native plugin. Claude Web uses the portable skill ZIP.
Both installation routes match the [website guide](https://gethealthsync.app/#install-claude).

## Claude Code

Run both commands in your terminal:

```bash
claude plugin marketplace add https://raw.githubusercontent.com/lukasosterheider/apple-health-for-ai-agents/main/claude/marketplace.json
claude plugin install apple-health-sync@healthsync
```

Restart Claude Code or run `/reload-plugins`. The plugin includes its runtime;
Python and pip are not required on the user's computer.

[`marketplace.json`](marketplace.json) and the npm-compatible package metadata in
[`npm/`](npm/) are served directly from this repository. Claude Code downloads the
self-contained package from
[GitHub Releases](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases)
and verifies its integrity. Installation no longer depends on website downloads.
The plugin ID and marketplace name are unchanged.

## Claude Web

1. Download [apple-health-sync-agent-skill.zip](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/latest/download/apple-health-sync-agent-skill.zip).
2. Enable Code execution under **Settings → Capabilities**.
3. Open **Customize → Skills → + Create skill → Upload a skill**.
4. Upload the ZIP and enable Apple Health Sync.

Upload the ZIP as downloaded, without extracting or repacking it. Its portable
source is in [`generic-skill/apple-health-sync`](../generic-skill/apple-health-sync).
See the [Claude Web guide](https://gethealthsync.app/#install-claude-web) for the illustrated steps.

## Start onboarding and Connect your iPhone

Start a fresh conversation in Claude and send:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

Approve initialization when prompted. Open
[Health Sync on your iPhone](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose QR Code Setup, scan the code, select Apple Health permissions, and complete
the first sync. Then ask Claude: `What Apple Health data can I access?`
