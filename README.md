# go-khard

`go-khard` is a keyboard-driven terminal address book for vdirsyncer vCard
collections. It uses the same dependencies and configuration file as
[`go-khal`](../go-khal), so calendars and address books can be configured once.

## Installation

Requirements:

- Local vdir contact data, commonly synchronized by `vdirsyncer`
- A terminal with alternate-screen and color support
- `$VISUAL` or `$EDITOR` for manual vCard editing (`vi` is the fallback)

### Install From Release Binaries

Download the archive for your platform from the
[GitHub releases page](https://github.com/hsanson/go-khard/releases).

Release artifacts are named by version, operating system, and architecture. For
example: `go-khard_v0.0.1_linux_amd64.tar.gz`,
`go-khard_v0.0.1_darwin_arm64.tar.gz`, and
`go-khard_v0.0.1_windows_amd64.zip`.

Linux x86-64 example:

```sh
curl -LO https://github.com/hsanson/go-khard/releases/download/v0.0.1/go-khard_v0.0.1_linux_amd64.tar.gz
curl -LO https://github.com/hsanson/go-khard/releases/download/v0.0.1/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf go-khard_v0.0.1_linux_amd64.tar.gz
install -m 0755 go-khard ~/.local/bin/go-khard
```

Replace `v0.0.1` with the release you want to install.

### Install From Source

Source installs require Go 1.24.2 or newer:

```sh
go install github.com/hsanson/go-khard@latest
```

For a local checkout:

```sh
make install
```

Ensure the Go binary directory, usually `~/go/bin`, is in your `PATH`.

## Quick start

```sh
go-khard config from-vdirsyncer
go-khard
```

The default configuration is `~/.config/go-khal/config.json`. Only sources with
`"type": "addressbook"` are loaded. Use `--config PATH` to select another file.

The main view provides fuzzy search by name, email, or phone and displays name,
address book, email addresses, and phone numbers as columns.

| Key | Action |
| --- | --- |
| `j` / `k` | move |
| `/` | fuzzy search |
| `space` | select/unselect |
| `enter` | show contact |
| `a` / `e` | add/edit |
| `ctrl-e` | edit raw vCard in `$VISUAL`, `$EDITOR`, or `vi` |
| `c` / `x` | copy/move selected contacts |
| `d` | delete selected contacts |
| `M` | merge two or more selected contacts |
| `q` | quit |

Copy, move, and delete require confirmation. Merge combines list-valued fields,
prompts for scalar conflicts (`e` enters a custom value), opens a final review
form, and then saves to the selected address book before removing duplicates.

## Neomutt

Either invocation can be used as a neomutt `query_command`:

```muttrc
set query_command = "go-khard query '%s'"
# The shorthand is also accepted:
# set query_command = "go-khard '%s'"
```
