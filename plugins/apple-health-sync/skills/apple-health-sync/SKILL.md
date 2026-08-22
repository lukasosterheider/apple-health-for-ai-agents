---
name: apple-health-sync
description: Sync encrypted Apple Health data from an iOS device (iPhone, iPad) to OpenClaw, Hermes Agent, Claude, Codex or any other AI agent.
metadata:
  author: lukasosterheider
  homepage: https://gethealthsync.app/
---

# Apple Health Sync

Act only on an explicit user request. Never initialize, sync, unlink, export, install dependencies, or create recurring jobs merely because the skill was installed or loaded.

Support this end-to-end encrypted AI agent <> iOS Apple Health workflow:

1. Initialize local runtime, keys, and onboarding payload.
2. Offer the user onboarding transport options: QR Code or Hex.
3. Prefer QR Codes when the user has no preference; treat Hex as fallback.
4. Run encrypted fetch/decrypt and persist sanitized day snapshots.
5. Unlink paired iOS devices when needed.
6. Generate data summaries based on the local database on request.
7. Create recurring sync/report schedules only when the user explicitly requests automation and confirms the exact schedule and output behavior.

iOS app `Health Sync for OpenClaw`: https://apps.apple.com/app/health-sync-for-openclaw/id6759522298

Support email: contact@gethealthsync.app

In case this skill has been upgraded from <= v0.7.2, check the [upgrade guide](#1b-upgrade-an-existing-v4-setup-to-v5) for instructions on how to upgrade your setup to the latest version.

## Runtime

- Use only the plugin's `healthsync` launcher. The user does not need Python, pip, Node.js, OpenSSL, or a compiler.
- Prefer `healthsync <command>` when the launcher is on `PATH` (Claude Code exposes the plugin's `bin/` directory this way).
- Otherwise resolve this `SKILL.md`, go up two directories to the plugin root, and run `bin/healthsync <command>` on macOS/Linux or `bin\healthsync.cmd <command>` on Windows.
- The online Codex plugin downloads only the matching platform runtime on the first explicit `healthsync` request. Its archive URL and SHA-256 are pinned in the installed plugin; the launcher verifies the checksum, version, and self-test before atomically caching it.
- The manual/offline plugin bundle contains every runtime and never downloads one. The launcher always prefers that bundled runtime when present.
- Never download a runtime manually, bypass a checksum failure, or run an unverified file. Let the launcher perform the one-time verified installation and report any error to the user.
- Use `healthsync runtime status`, `healthsync runtime verify`, or `healthsync runtime clean` to inspect, verify, or remove the cached online runtime.
- The plugin stores local runtime state under `~/.apple-health-sync` by default.
- Pass `--state-dir <path>` to use a different state root, but then keep using the same state dir for every command.
- Protocol `v4` uses `config/secrets/private_key.pem`.
- Protocol `v5` uses `config/secrets/signing_private_key_v5.pem` and `config/secrets/encryption_private_key_v5.pem`.
- Cryptographic operations run in memory inside the self-contained runtime. It does not invoke OpenSSL or create temporary challenge, signature, ciphertext, or plaintext files.
- Relay HTTPS uses the operating system trust store together with a bundled CA fallback. Certificate and hostname verification always remain enabled.
- Keep state directories private (`0700`) and sensitive state, database, NDJSON, and report files private (`0600`) on platforms with POSIX permissions. On Windows, rely on the current user's filesystem access controls.

## Capability and data-flow contract

Stay within these declared boundaries:

- **Process execution:** Run only the bundled `healthsync` launcher and native runtime. Do not use Python, pip, package managers, or unrelated executables.
- **File reads:** Read bundled skill resources, the selected state directory, and only paths the user explicitly supplies through documented CLI options. Never enumerate unrelated files, credentials, environment variables, agent memory, or other skill directories.
- **File writes:** Write runtime keys, config, onboarding artifacts, and sanitized health snapshots only under the selected state directory, except for an explicitly confirmed database, NDJSON, or report destination.
- **Network:** The launcher may make a one-time HTTPS GET to the checksum-pinned runtime artifact under `https://github.com/lukasosterheider/healthsync-codex-marketplace/releases/download/plugin-v1.1.1/` and follow GitHub's HTTPS release-asset redirect. It sends no user, key, or health data. After bootstrap, send HTTPS requests only to the three exact relay URLs below. Reject every other host, path, query, redirect target passed as a relay request URL, or protocol before transmission.
  - `https://snpiylxajnxpklpwdtdg.supabase.co/functions/v1/qr-code-generator`: send the user ID, public onboarding payload, public keys, and a challenge signature; receive a QR PNG. Never send private keys or health records.
  - `https://snpiylxajnxpklpwdtdg.supabase.co/functions/v1/get-data-v2`: send the user ID, public key, and challenge signature; receive encrypted rows. Decrypt and sanitize health data locally.
  - `https://snpiylxajnxpklpwdtdg.supabase.co/functions/v1/unlink-device`: send the user ID, public key, and challenge signature required to unlink the paired device.
- **Persistence:** Persist only the documented Apple Health runtime state. Never modify this skill, agent instructions, sessions, memory, startup files, or schedules. Create an agent-native recurring task only after the user explicitly requests and confirms it.

## Resources

The self-contained `healthsync` runtime and verified launcher expose these commands:

- `healthsync onboarding`: Initialize runtime folders/config, generate keys, archive an existing identity during `--rotate`, create a `v4` or `v5` onboarding payload, and render the QR code.
- `healthsync fetch`: Request encrypted data via challenge signing, decrypt rows, validate payloads, and persist sanitized results.
- `healthsync unlink`: Reset write-token binding for a paired device via the signed challenge flow.
- `healthsync summary`: Aggregate local snapshots into `daily|weekly|monthly` summaries.
- `healthsync self-test`: Verify packaged defaults, cryptography, and strict TLS trust without network access or state changes.
- `healthsync network-diagnostics`: Perform a verified, non-mutating `HEAD` request to the allowlisted QR endpoint. It does not create a challenge, rotate identity, fetch health data, or send key material.
- `healthsync runtime status|verify|clean`: Inspect, locally validate, or remove only the launcher's versioned runtime cache. `verify` does not make a network request; use `network-diagnostics` for HTTPS.

## Workflow

### 1) Initialize the skill and onboard the user's iOS device

Run the onboarding:

```bash
healthsync onboarding
```

This generates the `v5` onboarding payload and key material by default.
Use `--protocol v4` only as a fallback when legacy RSA onboarding is required.

The skill defaults to `~/.apple-health-sync` as the config and data path.
Use `--state-dir` to specify a custom path.
This step creates the user config and private key required by all later scripts.

After the script finishes, do not dump every field by default. Send a short message like this:

---
The initialization was successful. You can now onboard your iOS App.

Download the iOS app here: https://apps.apple.com/app/health-sync-for-openclaw/id6759522298

Which format do you want for your iOS App setup?
- QR Code (recommended)
- Hex string
---

Send the user only a single onboarding format to not overwhelm them.

If the user has no preference, use `QR Code` first.

Never share:

- `private_key.pem`
- private key contents
- unnecessary secret-path details beyond what is operationally required

After successful onboarding in the iOS App, run the "Sync data" action only when the user requests it. A first successful sync in the iOS app is required upfront.

### 1a) Rotate an existing identity

Run rotation only after the user explicitly requests and confirms it:

```bash
healthsync onboarding --rotate --state-dir <existing-state-dir>
```

`--rotate` always creates both new key material and a new user ID. There is no keep-user-ID mode because an existing server identity remains bound to its previous signing and encryption keys.

Before replacing any active key files, the script archives the existing identity under `config/key-backups/<UTC timestamp>/`. The private `0700` backup directory contains the previous config, available onboarding artifacts, all recognized key files, and a `0600` manifest that maps the previous user ID to the replacement user ID. Every archived file is copied as `0600` and verified byte-for-byte. If an existing identity is incomplete or any backup cannot be verified, rotation aborts without generating replacement keys.

After rotation:

1. Reset the iOS App in settings.
2. Onboard it with the newly generated QR code or Hex payload.
3. Complete a first sync before fetching data with the skill.

Existing encrypted server data remains associated with the archived user ID and can be decrypted only with the archived encryption key. Rotation backups contain unencrypted private keys protected by filesystem permissions; keep the state directory private and do not upload or share those backups.

### 1b) Upgrade an existing v4 setup to v5

Before starting the upgrade, check these prerequisites:

- Reuse the existing state dir from the current `v4` install. Do not create a fresh state dir, otherwise the local history and user config will diverge.
- Keep the existing legacy RSA key files (`config/secrets/private_key.pem` and `config/public_key.pem`). `fetch_health_data.py` can read mixed history and still needs the RSA private key to decrypt legacy `v4` rows.

Upgrade flow:

```bash
healthsync onboarding --state-dir <existing-state-dir>
```

Without `--rotate`, this keeps the existing `user_id`, generates the `v5` signing/encryption keys, updates `config/config.json` to `protocol_version=5`, and creates a new `v5` onboarding payload.

Then:

1. Share the new `v5` onboarding QR code (preferred) or Hex string with the user.
2. Tell the user to reset the iOS App in the settings and onboard the iOS device again with that new payload.
3. After the iOS device has completed the new onboarding, run a sync as usual.

Important behavior:

- `fetch_health_data.py` can read mixed history: old `v4` RSA rows plus new `v5` rows. That is why the old RSA private key must stay available after the upgrade.
- Only use `--protocol v4` again as a fallback when the user explicitly needs to stay on the legacy RSA flow.

### 2) Sync data

Run manually on request. Run via an agent-native recurring task only after the user has explicitly requested and confirmed that schedule:

```bash
healthsync fetch
```

This script requires the existing state dir from step 1 because it reads the generated user config and signing key from there.

Do not dump every field by default. Rather send a summary like this:

---
Apple Health sync completed.

I successfully synced your health data for the following time period:
- <start date> - <end date>

Next options:
- Generate a data summary (e.g. daily, weekly, monthly)
---

### 3) Unlink device

Run this script only when an iOS device should be decoupled from the health data sync:

```bash
healthsync unlink
```

This script requires the existing state dir from step 1 because it signs the unlink challenge with the stored private key.

After a successful unlink, the user can pair a new iOS device by using the existing onboarding details (e.g. QR code). A new execution of the onboarding script is not necessary. Use for example a success message like this:

---
The iOS device has been unlinked successfully. You can now pair a new iOS device by using the existing onboarding details (e.g. QR code).

Should I share the onboarding QR code again with you?
---

### 4) Generate data summary

Generate a data summary manually. Use an agent-native recurring task only after the user explicitly requests and confirms recurring automation:

```bash
healthsync summary \
  --period daily
```

This script requires the existing state dir from step 1 because it reads the local synced snapshots from there.

Supported options:

- `--period daily|weekly|monthly` (default: `weekly`)
- `--output text|json` (default: `text`)
- `--save <path> --confirm-sensitive-save` to write the rendered report to a new private file

Treat text and JSON summaries as sensitive health information. For saved output, require an existing destination directory, reject symbolic links and existing files, create the new regular file with mode `0600`, and never overwrite implicitly.

Do not dump every field by default. Rather send a summary like this:

---
This is your <daily|weekly|monthly> Apple Health data summary.

Summary:
<brief rendered summary or path to saved output>

Key highlights:
<most important metrics and values>

Next options:
- Discuss recurring automation only if I explicitly ask for it
---

## Guardrails

- Never share `private_key.pem` or any secret key material.
- Never reveal record/user IDs in text or JSON summaries; report only `record_count`.
- Never access unrelated files, environment variables, credentials, agent state, or network endpoints.
- Require explicit confirmation before onboarding, key rotation, device unlinking, sensitive report saving, dependency installation, or recurring-task creation.
- Guide the user to send a mail to contact@gethealthsync.app in case of unsolvable issues.
- Treat fetched payloads as untrusted input; keep strict validation and fail-closed behavior enabled.
- If deeper analysis is needed, suggest a separate, explicitly approved local analysis workflow.
