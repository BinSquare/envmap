<p align="center">
  <img src="./logo.svg" alt="envmap logo" width="280">
</p>

<p align="center">
  <a href="https://github.com/binsquare/envmap/stargazers">
    <img src="https://img.shields.io/github/stars/binsquare/envmap?style=social" alt="GitHub stars">
  </a>
  <a href="https://github.com/binsquare/envmap/actions/workflows/release.yml">
    <img src="https://img.shields.io/github/actions/workflow/status/binsquare/envmap/release.yml?branch=main&label=release" alt="Release workflow status">
  </a>
  <a href="https://pkg.go.dev/github.com/binsquare/envmap">
    <img src="https://pkg.go.dev/badge/github.com/binsquare/envmap" alt="Go pkg Reference">
  </a>
  <a href="https://goreportcard.com/report/github.com/binsquare/envmap">
    <img src="https://goreportcard.com/badge/github.com/binsquare/envmap" alt="Go Report Card">
  </a>
  <a href="https://github.com/binsquare/envmap/blob/main/LICENSE">
    <img src="https://img.shields.io/github/license/binsquare/envmap" alt="License: Apache-2.0">
  </a>
</p>

# envmap

`envmap` keeps secrets out of your Git history by sourcing them from a provider (local encrypted store, AWS SSM, Vault, etc.) and injecting them directly into the target process. No `.env` files, no accidental commits, no “who has the latest .env?” in Slack.

## Why?

- `.env` files are easy to leak and hard to rotate across multiple engineers and machines.
- Most teams already have a secrets backend (or should); local dev is the messy part.
- `envmap` gives each repo a single, typed mapping from “env name → provider path” and a consistent `envmap run -- <cmd>` entrypoint.

## Installation

### Option 1: Go toolchain

```sh
go install github.com/binsquare/envmap@latest
```

### Option 2: Prebuilt binary (bash)

```sh
# installs to /usr/local/bin/envmap by default
curl -sSfL https://github.com/binsquare/envmap/releases/latest/download/envmap_$(uname -s)_$(uname -m).tar.gz \
  | tar -xz -C /usr/local/bin envmap

# (optional) verify checksum
curl -sSfL https://github.com/binsquare/envmap/releases/latest/download/envmap_$(uname -s)_$(uname -m).tar.gz.sha256 \
  | sha256sum --check -
```

If you install somewhere else, add that directory to your shell profile:

| Shell | Command                                              |
| ----- | ---------------------------------------------------- |
| bash  | `echo 'export PATH="$HOME/bin:$PATH"' >> ~/.bashrc`  |
| zsh   | `echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc`   |
| fish  | `set -Ux fish_user_paths $HOME/bin $fish_user_paths` |

Restart the shell (or reload the profile) and run `envmap --help` to verify.

## Quick start (local provider)

```sh
# 1. Generate encryption key (writes to ~/.envmap/key)
envmap keygen

# 2. Configure global provider (wizard; runs keygen if needed)
envmap init --global

# 3. Create project config (wizard)
envmap init

# 4. Add secrets
envmap set --env dev DATABASE_URL --prompt
envmap set --env dev STRIPE_KEY --prompt

# 5. Inspect/export/run
envmap env
eval $(envmap export --env dev)
envmap run -- npm start
```

Prefer the wizards, but manual config is supported. Use absolute paths (Go does not expand `~` or `${HOME}`).

<details>
<summary>Manual configuration reference</summary>

### Global config (`~/.envmap/config.yaml`)

```yaml
providers:
  local-dev:
    type: local-file
    path: /Users/<you>/.envmap/secrets.db
    encryption:
      key_file: /Users/<you>/.envmap/key
```

### Project config (`.envmap.yaml`)

```yaml
project: demo
default_env: dev
envs:
  dev:
    provider: local-dev
    prefix: demo/dev/
```

</details>

## Usage

- `envmap run -- <command>` – fetch secrets for an environment and exec the target process with those env vars (disk never sees them).
- `envmap env [--env <name>] [--raw]` – inspect secrets; masked by default, `--raw` reveals values.
- `envmap export [--env <name>] [--format plain|json]` – output suitable for `eval`, direnv, or tooling.
- `envmap get --env <name> KEY [--raw]` – read individual secrets.
- `envmap set --env <name> KEY (--prompt | --file PATH)` – write/update secrets without shell history.
- `envmap import PATH --env <name> [--delete]` – ingest existing `.env` files.
- `envmap keygen [-o PATH]` – create a 256-bit key for the local provider.
- `envmap validate` – confirm `.envmap.yaml` and global config reference defined providers.
- `envmap init` / `envmap init --global` – interactive project/global configuration.

### Use with direnv

```sh
# .envrc
eval "$(envmap export --env dev)"
```

Then run `direnv allow`. Every time you enter the directory, direnv will re-run `envmap export` and populate the shell with fresh secrets without touching disk.

## Configuration

### Global config (`~/.envmap/config.yaml`)

```yaml
providers:
  aws-dev:
    type: aws-ssm
    region: us-west-2
    profile: dev # optional, uses default credential chain

  vault-prod:
    type: vault
    address: https://vault.internal:8200
    mount: secret # default: secret

  local:
    type: local-file
    path: ~/.envmap/secrets.db
    encryption:
      key_file: ~/.envmap/key # must be chmod 600
      # or: key_env: ENVMAP_KEY
```

### Project config (`.envmap.yaml`)

```yaml
project: myapp
default_env: dev

envs:
  dev:
    provider: aws-dev
    path_prefix: /myapp/dev/

  staging:
    provider: aws-dev
    path_prefix: /myapp/staging/

  local:
    provider: local
    prefix: myapp/
```

## Built-in providers (WIP)

| Type                 | Auth                       | Notes                                                   |
| -------------------- | -------------------------- | ------------------------------------------------------- |
| `aws-ssm`            | IAM (profile/env/instance) | Requires `path_prefix`. Uses SecureString.              |
| `aws-secretsmanager` | IAM                        | Full secret names. JSON secrets expanded to `name/key`. |
| `gcp-secretmanager`  | ADC or service account     | Reads latest version. Adds version on write.            |
| `vault`              | Token (env/config)         | KV v2. Configurable mount path and namespace.           |
| `onepassword`        | Connect server             | Requires `connect_host`. Items matched by title.        |
| `doppler`            | Service token              | Read-only; writes require Doppler CLI.                  |
| `local-file`         | AES-256-GCM                | Key from file (0600) or env var. For local dev.         |

## Security Model

- Secrets never touch disk during normal operation. (unless you choose local)
- `set --prompt` and `set --file` avoid shell history
- Values masked by default in `env` and `get` output
- `export` outputs to stdout only, no file writing

### Local provider hardening

| Layer            | Implementation                      |
| ---------------- | ----------------------------------- |
| Encryption       | AES-256-GCM (authenticated)         |
| Key derivation   | HKDF-SHA256 with purpose binding    |
| Nonce            | Random 96-bit per write             |
| File permissions | Key: 0600, Secrets: 0600, Dir: 0700 |
| Locking          | Process-safe file locks (`flock`)   |
| Atomic writes    | Write to temp + rename (crash-safe) |
| Minimum key      | 16 bytes enforced                   |

Generate keys with `envmap keygen` (256 bits from crypto/rand). Store the key file outside your repository.

## Contributions

Contributions and bug reports are welcome—open an issue or submit a PR if you find a bug.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](./LICENSE) for details.
