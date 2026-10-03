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

### Omarchy themes

On Linux, the `go-khard` and `go-khard add-email` TUIs automatically use the
active Omarchy palette from
`~/.local/state/omarchy/current/theme/colors.toml`. Theme changes are applied
while the TUI is running. Application surfaces keep the terminal's default
background. A missing or invalid startup palette uses the built-in styles; an
invalid runtime update keeps the last valid palette until it is repaired. Other
platforms always use the built-in styles.

The main view displays each contact's name, address book, preferred email, and
preferred phone. Press `/` to search names, email addresses, and phone numbers
with fuzzy matching. While searching, use `ctrl-j`/`ctrl-k` or the arrow keys to
move through results. Press `enter` to keep the filter or `esc` to clear it.

| Key | Action |
| --- | --- |
| `j` / `k` or `up` / `down` | move |
| `ctrl-f` / `ctrl-b` or `page-down` / `page-up` | move one page |
| `/` | search contacts |
| `tab` / `shift-tab` | cycle through All and each address book |
| `enter` | edit contact |
| `n` | create contact |
| `space` | select/unselect contact |
| `c` | copy the current or selected contacts |
| `m` | move selected contacts |
| `M` | merge two or more selected contacts |
| `d` | delete selected contacts |
| `q` / `esc` | quit |
| `?` | show shortcuts for the current screen |

Move, merge, and delete appear only when enough contacts are selected. Copy,
move, and delete require confirmation. Merge combines list-valued fields,
prompts for scalar conflicts, opens a review form, and asks for final
confirmation before saving the merged contact and removing its sources.

The footer lists every shortcut available in the current screen or dialog.
Field dialogs contain no shortcut labels.

The add, edit, and merge-review screens use a sectioned item editor covering
identity and structured names, kind, nicknames, dates, organizations, titles,
roles, typed phone numbers and emails, structured postal addresses, categories,
webpages, private `X-*` properties, and notes. Use `j`/`k`, the arrow keys, or
tab/shift-tab to navigate. Highlighted `󰐕 Add …` buttons append repeatable
values. Save and Cancel buttons finish the contact form.

All dialogs use 60% of the terminal width. Field overlays reserve action slots
for Apply, Cancel, and the optional Delete button. Existing repeatable items
have a red Delete button. Tab/down and shift-tab/up move
through fields and buttons; Enter on the final field applies the form directly.
Select fields also use `j`/`k`, `h`/`l`, or left/right to change options. In the
Note field, up/down moves the cursor and `ctrl-enter` inserts a newline.

Birthday and Anniversary use a single-date calendar with keyboard and mouse
navigation, Today, Clear, Apply, and Cancel controls. Empty and legacy vCard
dates remain unchanged until a date is explicitly selected or cleared.

Mouse input supports wheel navigation, address-book cycling, contact and field
opening, and every visible button. Ctrl-click selects or unselects a contact.

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
