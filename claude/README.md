# Apple Health Sync for Claude

## Claude Code

Run in your terminal:

```bash
claude plugin marketplace add https://raw.githubusercontent.com/lukasosterheider/apple-health-for-ai-agents/main/claude/marketplace.json
claude plugin install apple-health-sync@healthsync
```

Restart Claude Code or run `/reload-plugins`. The plugin includes the Health Sync
executable; Python is not required.

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
