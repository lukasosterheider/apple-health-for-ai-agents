# Portable Apple Health Sync skill

Use this Agent Skills package with Hermes Agent or another compatible agent.
The installation routes below match the
[website guide](https://gethealthsync.app/#install-hermes-agent).

## Interactive installation

With Node.js 18 or newer installed, run:

```bash
npx skills add https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/latest/download/apple-health-sync-agent-skill.zip
```

Select Hermes Agent or your compatible agent when prompted, then choose global
or project scope. The portable scripts need Python 3 and the dependencies in
[`requirements.txt`](apple-health-sync/requirements.txt).

## Manual ZIP installation

Download [apple-health-sync-agent-skill.zip](https://github.com/lukasosterheider/apple-health-for-ai-agents/releases/latest/download/apple-health-sync-agent-skill.zip)
and use your agent's skill import or installation flow. The ZIP contains one
`apple-health-sync` folder with `SKILL.md` and its runtime resources. For Claude
Web's upload steps, see [`claude/README.md`](../claude/README.md).

The download link follows the latest stable GitHub release. Each release also
provides `apple-health-sync-agent-skill.zip.sha256`. For a fixed version, use that
release's download link instead of `releases/latest/download`.

## Connect your iPhone

Start a fresh agent conversation and send:

```text
Use Apple Health Sync and start onboarding with a QR code.
```

Approve initialization when prompted. Open
[Health Sync on your iPhone](https://apps.apple.com/app/health-sync-for-openclaw/id6759522298),
choose QR Code Setup, scan the code, select Apple Health permissions, and complete
the first sync. Then ask your agent: `What Apple Health data can I access?`

## Develop the shared source

[`apple-health-sync/`](apple-health-sync) is the editable source used to build the
portable ZIP, OpenClaw variant, and native plugin runtimes. From the repository root:

```bash
python3 -m venv .venv
.venv/bin/python -m pip install -r generic-skill/apple-health-sync/requirements.txt
.venv/bin/python -m unittest discover -s generic-skill/apple-health-sync/scripts -p 'test_*.py' -v
```

On Windows, use `.venv\Scripts\python.exe`. See
[`SKILL.md`](apple-health-sync/SKILL.md) for runtime usage and
[`references/config.md`](apple-health-sync/references/config.md) for local storage.
Never commit private keys, local configuration, QR onboarding artifacts, or health data.
