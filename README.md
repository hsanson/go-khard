# go-khard

`go-khard` is a keyboard-driven terminal address book for vdirsyncer vCard
collections. It uses the same dependencies and configuration file as
[`go-khal`](../go-khal), so calendars and address books can be configured once.

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
