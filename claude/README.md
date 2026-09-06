# Apple Health Sync for Claude

## Claude Code

Requires Claude Code **2.1.224 or later**. Run `claude --version` to check.
The plugin installs directly from a SHA-256-verified release ZIP; Node.js, npm,
Git and Python are not required for this installation.

Run in your terminal:

```bash
claude plugin marketplace add https://raw.githubusercontent.com/lukasosterheider/apple-health-for-ai-agents/main/claude/marketplace.json
claude plugin install apple-health-sync@healthsync
```

Restart Claude Code or run `/reload-plugins`. The plugin includes the Health Sync
executable; Python is not required.

## Update an existing Claude Code installation

Refresh the marketplace before updating the plugin:

```bash
claude plugin marketplace update healthsync
claude plugin update apple-health-sync@healthsync
```

Restart Claude Code or run `/reload-plugins`. Updating from the previous npm-based
package keeps the plugin name and your existing `~/.apple-health-sync` data.
If Claude reports an unsupported source type, update Claude Code before retrying.

## Claude Web

1. Download [apple-health-sync-agent-skill.zip](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/latest/download/apple-health-sync-agent-skill.zip).
2. Enable **Code execution** under **Settings → Capabilities**.
3. Open **Customize → Skills → + Create skill → Upload a skill**.
4. Upload the ZIP without extracting it, then enable Apple Health Sync.

Your Claude environment must allow outbound HTTPS and retain the skill's private
state between uses. [Illustrated guide](https://gethealthsync.app/#install-claude-web).

## Start onboarding and Connect your iPhone

Start a fresh conversation and send:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

Open [Health Sync on your iPhone](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose **QR Code Setup**, scan the code, grant Apple Health permissions and complete
your first sync. Then ask Claude: `Fetch my health data and summarize this week.`
