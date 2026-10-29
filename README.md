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
example: `go-khard_v0.3.0_linux_amd64.tar.gz`,
`go-khard_v0.3.0_darwin_arm64.tar.gz`, and
`go-khard_v0.3.0_windows_amd64.zip`.

Linux x86-64 example:

```sh
export VERSION=0.3.0
curl -LO https://github.com/hsanson/go-khard/releases/download/v${VERSION}/go-khard_v${VERSION}_linux_amd64.tar.gz
curl -LO https://github.com/hsanson/go-khard/releases/download/v${VERSION}/SHA256SUMS
sha256sum -c SHA256SUMS --ignore-missing
tar -xzf go-khard_v${VERSION}_linux_amd64.tar.gz
install -m 0755 go-khard ~/.local/bin/go-khard
```

Replace `VERSION` with the release you want to install.

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

Print the application and build toolchain versions:

```sh
go-khard version
```

## Quick start

```sh
go-khal config from-vdirsyncer
go-khard
```

`go-khard` reads `${XDG_CONFIG_HOME:-$HOME/.config}/khal/config`. Khal calendar
sections with `type = birthdays` are editable addressbooks; other calendar
types are ignored. Use `--config PATH` to read a specific khal or JSON file.
go-khard never modifies configuration.

If the khal config is absent, go-khard falls back to the deprecated
`~/.config/go-khal/config.json`. JSON configuration support will be removed in
a future release.

### Omarchy themes

On Linux, the `go-khard` and `go-khard add-email` TUIs automatically use the
active Omarchy palette from
`~/.local/state/omarchy/current/theme/colors.toml`. Theme changes are applied
while the TUI is running. Application surfaces keep the terminal's default
background. A missing or invalid startup palette uses the built-in styles; an
invalid runtime update keeps the last valid palette until it is repaired. Other
platforms always use the built-in styles.

The main workspace keeps contact names in the left pane and the selected
contact's read-only details in the right pane. Details include the address book
and every populated editor field. Notes wrap without truncation. The vCard path
stays hidden; use its Copy button to copy it to the system clipboard.

Press `/` to search names, email addresses, and phone numbers with fuzzy
matching. While searching, use `ctrl-j`/`ctrl-k` or the arrow keys to move
through results. Press `enter` to keep the filter or `esc` to clear it.

| Key | Action |
| --- | --- |
| `tab` / `shift-tab` or `h` / `l` | move between Contacts and Details |
| `j` / `k` or `up` / `down` | move through contacts or scroll Details |
| `ctrl-f` / `ctrl-b` or `page-down` / `page-up` | move or scroll one page |
| `[` / `]` | cycle through All and each address book |
| `/` | search contacts |
| `enter` | focus Details from Contacts; edit from Details |
| `e` | edit the current contact |
| `n` | create a contact |
| `space` | select or unselect a contact |
| `c` | copy selected contacts |
| `m` | move selected contacts |
| `M` | merge two or more selected contacts |
| `d` | delete selected contacts |
| `q` | quit from Contacts or Details |
| `esc` | return to Contacts from Details; quit from Contacts |
| `?` | show shortcuts for the current screen |

The Contacts pane shows Move, Copy, and Merge buttons when their selection
requirements are met. Copy and Move require at least one selected contact; Merge
requires at least two. Move stays hidden when no other address book exists.
Copy, move, and delete require confirmation. Merge combines list-valued fields,
prompts for scalar conflicts, opens a review form, and asks for final
confirmation before saving the merged contact and removing its sources.

The footer lists every shortcut available in the current pane or dialog. Field
dialogs contain no shortcut labels.

The add, edit, and merge-review screens use a sectioned item editor covering
identity and structured names, kind, nicknames, dates, organizations, titles,
roles, typed phone numbers and emails, structured postal addresses, categories,
webpages, private `X-*` properties, and notes. Normal forms and workflows open
in the right pane while the Contacts pane remains visible. Use `j`/`k`, the
arrow keys, or tab/shift-tab to navigate. Highlighted `󰐕 Add …` buttons append
repeatable values. Save and Cancel finish every contact form; editing an
existing contact also shows Delete. `go-khard add-email` keeps its full-screen
form and exits after save or cancel.

Field overlays reserve action slots for Apply, Cancel, and the optional Delete
button. Existing repeatable items have a red Delete button. Enter advances
through multi-field overlays. On the final valid field, Enter focuses Apply;
press Enter again to apply the change. Invalid fields retain focus and show
their validation error. Tab/down and shift-tab/up provide the same navigation.
Select fields also use `j`/`k`, `h`/`l`, or left/right to change options. In
the Note field, up/down moves the cursor and `ctrl-enter` inserts a newline.
Action buttons remain on one row when they fit; Delete moves to a right-aligned
second row only when the dialog is too narrow.

Birthday and Anniversary use a single-date calendar with keyboard and mouse
navigation. The `‹`/`›` buttons move one month and `«`/`»` move one year;
`ctrl+k`/`ctrl+j` and `ctrl+h`/`ctrl+l` provide the same keyboard navigation.
Apply and Cancel appear below the calendar. Clear occupies the destructive
bottom-right action, clears the date, and closes the calendar immediately.
Empty and legacy vCard dates remain unchanged until a date is selected or
cleared.

Mouse input supports wheel navigation, pane focusing, address-book cycling,
contact and field opening, and every visible button. Click the current contact
again to focus Details. Ctrl-click selects or unselects a contact.

Phone and email entries allow multiple types, while address entries use a
single type. Extra type labels can be added to the shared khal configuration:

```ini
[go-khard]
phone_types = satellite
email_types = school
address_types = vacation
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
