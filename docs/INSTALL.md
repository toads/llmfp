# Installation guide

## Verify an installation

An installation is complete when this command works in a new terminal:

```bash
llmfp version
```

The executable is named `llmfp` on macOS and Linux and `llmfp.exe` on Windows.

## Option 1: prebuilt release

Prebuilt releases do not require Go. Download the archive matching the machine
from the [GitHub releases page](https://github.com/toads/llmfp/releases):

| Platform | Typical archive |
|---|---|
| Apple Silicon macOS | `llmfp_Darwin_arm64.tar.gz` |
| Intel macOS | `llmfp_Darwin_x86_64.tar.gz` |
| x86-64 Linux | `llmfp_Linux_x86_64.tar.gz` |
| ARM64 Linux | `llmfp_Linux_arm64.tar.gz` |
| x86-64 Windows | `llmfp_Windows_x86_64.zip` |
| ARM64 Windows | `llmfp_Windows_arm64.zip` |

The release also contains `checksums.txt`. Verify the downloaded archive before
extracting it when the platform provides a SHA-256 utility.

On macOS or Linux, place the extracted executable in a user-owned bin directory:

```bash
mkdir -p "$HOME/.local/bin"
install -m 0755 llmfp "$HOME/.local/bin/llmfp"
export PATH="$HOME/.local/bin:$PATH"
llmfp version
```

To make the PATH change persistent, add this line to the shell configuration:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Use `$HOME/.zshrc` for the usual macOS zsh setup or `$HOME/.bashrc` for the
usual Linux bash setup, then open a new terminal.

On Windows, extract `llmfp.exe` to a stable directory such as
`%USERPROFILE%\bin`, add that directory to the user `Path` in Windows Settings,
and open a new PowerShell window.

## Option 2: `go install`

Go 1.25.12 or newer is required:

```bash
go version
go install github.com/toads/llmfp/cmd/llmfp@latest
```

Go selects the install directory as follows:

1. `go env GOBIN`, when it is non-empty.
2. Otherwise, the `bin` directory under `go env GOPATH`, normally
   `$HOME/go/bin` on macOS/Linux or `%USERPROFILE%\go\bin` on Windows.

Go does not automatically add this directory to the operating system or shell
`PATH`.

### macOS and Linux

Locate the executable and test it by absolute path:

```bash
GO_BIN="$(go env GOBIN)"
[ -n "$GO_BIN" ] || GO_BIN="$(go env GOPATH)/bin"

echo "$GO_BIN"
ls -l "$GO_BIN/llmfp"
"$GO_BIN/llmfp" version
```

If that works, persist the directory for zsh:

```bash
printf '\nexport PATH="%s:$PATH"\n' "$GO_BIN" >> "$HOME/.zshrc"
source "$HOME/.zshrc"
hash -r
llmfp version
```

For bash, replace `$HOME/.zshrc` with `$HOME/.bashrc`. If a shell still cannot
find the executable, open a new terminal and run:

```bash
command -v llmfp
```

### Windows PowerShell

Locate the executable:

```powershell
$goBin = go env GOBIN
if (-not $goBin) { $goBin = Join-Path (go env GOPATH) "bin" }

Write-Host "llmfp was installed in: $goBin"
& (Join-Path $goBin "llmfp.exe") version
```

If the absolute-path command works, add the directory to the current process
and the user-level `Path`:

```powershell
$currentUserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if (($currentUserPath -split ";") -notcontains $goBin) {
    [Environment]::SetEnvironmentVariable(
        "Path",
        ($currentUserPath.TrimEnd(";") + ";" + $goBin),
        "User"
    )
}
$env:Path += ";$goBin"
llmfp version
```

Open a new PowerShell window after changing the persistent user `Path`.

## `command not found` checklist

If `go install` printed no error but the command is unavailable:

1. Run the executable by its absolute path using the commands above.
2. Confirm that its directory, not the executable path itself, is in `PATH`.
3. Open a new terminal or refresh the shell command cache with `hash -r`.
4. Check whether `GOBIN` is customized with `go env GOBIN`.
5. Check whether multiple Go installations use different `GOPATH` values.
6. Do not use `sudo go install`; it may install into root's Go environment.

If the executable is not present in the reported directory, rerun
`go install` without suppressing its output and resolve that installation error
before changing `PATH`.
