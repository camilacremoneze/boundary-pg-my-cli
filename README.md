# pgcli-boundary

A desktop GUI (Go + [Fyne](https://fyne.io/)) that collapses the entire workflow of connecting to a PostgreSQL, MySQL, or MariaDB database behind [HashiCorp Boundary](https://www.boundaryproject.io/). Many sessions from different environments can be managed in one place, and the app handles token management and session lifecycle for you.

```
Select environment → Login with SSO → Pick target → pgcli / mycli opens
```

---

## Prerequisites

| Tool | Install |
|------|---------|
| Go ≥ 1.21 | https://go.dev/dl/ |
| `boundary` CLI | https://developer.hashicorp.com/boundary/downloads |
| `pgcli` | `brew install pgcli` |
| `mycli` *(MySQL/MariaDB only)* | `brew install mycli` |
| Xcode CLT *(macOS)* | `xcode-select --install` |

---

## Configuration

```bash
cp .env.example .env
$EDITOR .env
```

| Variable | Description |
|----------|-------------|
| `BOUNDARY_ENVS` | Comma-separated `label=https://controller-url=amoidc_ID` triples |
| `BOUNDARY_DEFAULT_ENV` | Label pre-selected on startup |
| `BOUNDARY_USER` | SSO login hint (email) — optional |

To find your `amoidc_*` auth method ID:

```bash
boundary auth-methods list -addr https://your-boundary-controller.example.com
```

---

## Run

```bash
make deps    # download dependencies
make run     # build and launch
```

---

## Usage

1. **Login tab** — select an environment and click **Login with SSO**. A browser SSO flow opens; after approval the app switches to the Targets tab automatically. You can also paste an existing `at_xxxx…` token directly.

2. **Targets tab** — fuzzy-search the target list and click **Connect**. A Boundary session is established and `pgcli` or `mycli` opens automatically.

3. **Sessions tab** — each active session shows the host, port, and database. From here you can open a new CLI window, copy the DSN, or kill the session.

---

## Docker

The image bundles the app, `boundary` CLI, and `pgcli`. The GUI runs on the host display via X11 forwarding.

```bash
# macOS (requires XQuartz with "Allow connections from network clients" enabled)
xhost +127.0.0.1 && export DISPLAY=host.docker.internal:0

# Linux
xhost +local:docker

make docker-run
```

---

## License

[MIT](LICENSE)
