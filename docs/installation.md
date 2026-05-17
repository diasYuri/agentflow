# Installation

AgentFlow distributes the `agentflow` CLI and the `agentflowd` daemon through GitHub Releases and Homebrew.

Choose the path that fits your environment.

## Requirements

- macOS, Linux, or Windows (WSL recommended for Windows)
- `codex`, `claude`, or `pi` on `PATH` when a workflow uses `kind: agent`

## Direct download

1. Visit the [GitHub Releases](https://github.com/diasYuri/agentflow/releases) page.
2. Download the archive for your platform:
   - macOS Apple Silicon: `agentflow-<version>-darwin-arm64.tar.gz`
   - macOS Intel: `agentflow-<version>-darwin-amd64.tar.gz`
   - Linux ARM64: `agentflow-<version>-linux-arm64.tar.gz`
   - Linux AMD64: `agentflow-<version>-linux-amd64.tar.gz`
3. Extract the archive. It contains two binaries:
   - `agentflow` — the CLI
   - `agentflowd` — the local daemon
4. Move both binaries to a directory on your `PATH`, for example:

   ```bash
   tar -xzf agentflow-<version>-darwin-arm64.tar.gz
   sudo mv agentflow-<version>-darwin-arm64/agentflow agentflow-<version>-darwin-arm64/agentflowd /usr/local/bin/
   ```

## Homebrew

### Install

```bash
brew tap diasYuri/agentflow https://github.com/diasYuri/agentflow
brew install agentflow
```

The formula installs both `agentflow` and `agentflowd` into your Homebrew prefix and registers shell completions automatically.

### Upgrade

```bash
brew update
brew upgrade agentflow
```

### Uninstall

```bash
brew uninstall agentflow
```

## Checksum verification

Every release publishes a `SHA256SUMS.txt` file. After downloading an archive, verify its integrity:

```bash
curl -LO https://github.com/diasYuri/agentflow/releases/download/<version>/SHA256SUMS.txt
sha256sum -c SHA256SUMS.txt
```

On macOS you can use `shasum -a 256` instead of `sha256sum`:

```bash
shasum -a 256 -c SHA256SUMS.txt
```

## Post-install validation

Run `doctor` to confirm the installation and environment:

```bash
agentflow doctor
```

The report checks:

- `agentflow` and `agentflowd` binaries
- daemon status
- data directory permissions
- optional provider binaries (`codex`, `claude`)
- local workflow validity

For JSON output suitable for automation:

```bash
agentflow doctor --json
```

## Shell completions

If you installed via Homebrew, completions are already registered for `bash`, `zsh`, and `fish`.

For a manual installation, generate and source the script for your shell:

```bash
# Bash
source <(agentflow completion bash)

# Zsh
source <(agentflow completion zsh)

# Fish
agentflow completion fish | source
```

To make completions persistent, add the command to your shell profile (`~/.bashrc`, `~/.zshrc`, or `~/.config/fish/config.fish`).

## Desktop app

The desktop application is distributed separately. See [docs/desktop.md](desktop.md) for build instructions and distribution notes.
