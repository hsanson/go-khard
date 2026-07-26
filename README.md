# go-khard

`go-khard` is a keyboard-driven terminal address book for vdirsyncer vCard
collections. It uses the same dependencies and configuration file as
[`go-khal`](https://github.com/hsanson/go-khal), so calendars and address books can be configured once.

> [!WARNING]
> go-khard is alpha software. Use it at your own risk: contact editing, moving,
> merging, and deletion modify vCard files directly and may cause data loss.
> Keep backups or use versioned/synchronized addressbooks before using it with
> important contacts.

## Installation

Requirements:

- Local vdir contact data, commonly synchronized by `vdirsyncer`
- A terminal with alternate-screen and color support

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
| `b` | filter by addressbook |
| `space` | select/unselect |
| `enter` | show contact |
| `a` / `e` | add/edit |
| `c` / `x` | copy/move selected contacts |
| `ctrl-d` | delete selected contacts |
| `M` | merge two or more selected contacts |
| `q` | quit |

Copy, move, and delete require confirmation. Merge combines list-valued fields,
prompts for scalar conflicts (`e` enters a custom value), opens a final review
form, and then saves to the selected address book before removing duplicates.

The add, edit, and merge-review screens use a sectioned item editor covering
the complete khard contact template: identity and structured names, kind,
nicknames, dates, organizations, titles and roles, typed phone numbers and
emails, structured postal addresses, categories, webpages, private `X-*`
properties, and notes. Use `j`/`k` to navigate, `enter` to open a field popup,
the highlighted `+ Add …` rows to append repeatable values, `ctrl+d` to remove a
repeatable entry, and `ctrl+s` to save.

Phone and email entries allow multiple types, while address entries use a
single type. Extra type labels can be added to the shared configuration:

```json
{
  "contact_types": {
    "phone": ["satellite"],
    "email": ["school"],
    "address": ["vacation"]
  }
}
```

These values extend, rather than replace, the built-in type choices.

## Neomutt

Either invocation can be used as a neomutt `query_command`:

```muttrc
set query_command = "go-khard query '%s'"
# The shorthand is also accepted:
# set query_command = "go-khard '%s'"
```

To add the sender of the current message, bind a key to pipe the full message to
`go-khard add-email`:

```muttrc
macro index,pager A "<pipe-message>go-khard add-email<enter>" "add sender to contacts"
```

go-khard searches for contacts with a similar name or email address. If it
finds any, choose one to merge with the sender or choose `Create new`. The
merge and contact forms close automatically after a successful save.
