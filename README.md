# envmap

Injects secrets from external providers into process environments at runtime. No `.env` files committed, no secrets in shell history.

## Installation

```sh
go install github.com/binsquare/envmap@latest
```

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│  .envmap.yaml (per-project)                                  │
│  - Maps environments to providers                            │
│  - Defines key prefixes for namespacing                      │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  ~/.envmap/config.yaml (global)                              │
│  - Provider credentials and configuration                    │
│  - Shared across projects                                    │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  Provider Interface                                          │
│  Get(name) │ List(prefix) │ Set(name, value)                 │
├──────────────────────────────────────────────────────────────┤
│  aws-ssm │ aws-secretsmanager │ gcp-secretmanager │ vault   │
│  onepassword │ doppler │ local-file                          │
└──────────────────────────────────────────────────────────────┘
```

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

## Usage

### Run with injected environment

```sh
envmap run -- npm start
envmap run --env staging -- ./bin/server
```

Secrets are injected via the process environment. Nothing written to disk.

### Inspect secrets (human)

```sh
envmap env                    # masked, for debugging
envmap env --env prod --raw   # unmasked (use with care)
```

### Export secrets (machine)

```sh
# Shell eval (direnv, scripts)
eval $(envmap export --env dev)

# JSON for tooling
envmap export --env dev --format json | jq .
```

`export` outputs unmasked values to stdout for composition with other tools. No file writing.

### Get/set individual secrets

```sh
# Get
envmap get --env dev DB_URL
envmap get --env dev DB_URL --raw

# Set (never use command-line args for values)
envmap set --env dev DB_URL --prompt
envmap set --env dev DB_URL --file /tmp/secret.txt
```

### Import from `.env` files

```sh
envmap import .env --env dev
envmap import .env --env dev --delete  # delete after import
```

Parses the file, writes each key to the provider.

### Local storage setup

```sh
# Generate secure encryption key
envmap keygen

# Or specify a custom path
envmap keygen -o ~/.envmap/myproject-key
```

Then configure `~/.envmap/config.yaml`:

```yaml
providers:
  local:
    type: local-file
    path: ~/.envmap/secrets.db
    encryption:
      key_file: ~/.envmap/key
```

### Diagnostics

```sh
envmap validate  # validate configuration
envmap init   # interactive project setup
```

## Providers

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

- Secrets never touch disk during normal operation
- `set --prompt` and `set --file` avoid shell history
- Values masked by default in `env` and `get` output
- `export` outputs to stdout only, no file writing

### Local provider security

| Layer            | Implementation                      |
| ---------------- | ----------------------------------- |
| Encryption       | AES-256-GCM (authenticated)         |
| Key derivation   | HKDF-SHA256 with purpose binding    |
| Nonce            | Random 96-bit per write             |
| File permissions | Key: 0600, Secrets: 0600, Dir: 0700 |
| Locking          | Process-safe file locks (`flock`)   |
| Atomic writes    | Write to temp + rename (crash-safe) |
| Minimum key      | 16 bytes enforced                   |

Generate keys with `envmap keygen` (256 bits from crypto/rand). Store the key file:

- Outside your repository
- In a secure backup
- Never commit to version control

## Adding Providers

Providers self-register via `init()`. Create a new file in `provider/`:

```go
package provider

func init() {
    Register(Info{
        Type:           "my-backend",
        Description:    "My secret backend",
        Factory:        newMyBackend,
        RequiredFields: []string{"endpoint"},
    })
}

func newMyBackend(envCfg EnvConfig, cfg ProviderConfig) (Provider, error) {
    // ...
}

func (p *myBackend) Get(ctx context.Context, name string) (string, error) { ... }
func (p *myBackend) List(ctx context.Context, prefix string) (map[string]string, error) { ... }
func (p *myBackend) Set(ctx context.Context, name, value string) error { ... }
```

See `provider/TEMPLATE.go.example` for a complete example.

## Project Structure

```
├── main.go           # CLI commands (cobra)
├── config.go         # Config loading, backward compat for source→provider
├── env.go            # Secret collection, provider instantiation
├── provider/
│   ├── provider.go   # Interface + registry
│   ├── config.go     # Provider-specific types
│   ├── aws_ssm.go
│   ├── vault.go
│   └── ...
└── ...
```

## Backward Compatibility

Config files using the old `source`/`sources` naming continue to work. Internally mapped to `provider`/`providers`.

## License

MIT
