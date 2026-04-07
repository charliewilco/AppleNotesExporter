# AppleNotesExporter

`apple-notes-md` is a macOS CLI that exports Apple Notes to Markdown.

It talks to Notes through `osascript`, converts the note HTML to Markdown in Go, and writes one `.md` file per note using your Notes account and folder structure.

## What it does

- Exports notes to Markdown with YAML frontmatter
- Mirrors Notes accounts and folders by default
- Supports filtering by folder, title substring, and modified date
- Skips existing files unless `--overwrite` is set
- Supports dry runs and verbose output
- Writes inline `data:` images to a local `assets/` folder beside each note

## Requirements

- macOS
- Notes.app installed and accessible
- Go 1.23 or newer if you are building from source

On first run, macOS may prompt your terminal app for permission to control Notes. If access is denied, export will fail.

## Install

Install the latest version directly:

```bash
go install github.com/charliewilco/AppleNotesExporter@latest
```

Install from a local checkout:

```bash
go install .
```

## Development

This repo uses [`just`](https://github.com/casey/just) for local tasks.

```bash
just build
just install
just lint
```

## Usage

```bash
apple-notes-md export [flags]
```

### Flags

- `-o, --output string` Output directory. Default: `~/Downloads/AppleNotesExport`
- `-f, --folder string` Export only notes in this folder with an exact match
- `-q, --query string` Export only notes whose title contains this substring
- `--since string` Export only notes modified after this date. Accepts RFC3339 or `YYYY-MM-DD`
- `--flat` Write all notes into a single directory instead of account/folder paths
- `--overwrite` Replace existing files instead of skipping them
- `--no-images` Skip writing image assets
- `--dry-run` Print what would be exported without writing files
- `-v, --verbose` Print per-note export details to stderr
- `--workers int` Number of concurrent image workers. Default: `4`

## Examples

Export everything:

```bash
apple-notes-md export
```

Preview an export without writing files:

```bash
apple-notes-md export --query "project" --since 2024-03-15 --dry-run
```

Export one folder to a custom destination:

```bash
apple-notes-md export --folder Work --output ~/Desktop/NotesExport
```

Flatten the output and overwrite existing files:

```bash
apple-notes-md export --flat --overwrite
```

Skip image extraction:

```bash
apple-notes-md export --no-images
```

## Output

By default, exported notes are written to:

```text
~/Downloads/AppleNotesExport/{account}/{folder}/{note-title}.md
```

Example:

```text
~/Downloads/AppleNotesExport/iCloud/Work/Weekly Plan.md
```

Each note starts with frontmatter like this:

```yaml
---
title: "Weekly Plan"
created: 2024-03-15T10:30:00Z
modified: 2024-11-02T14:22:00Z
folder: "Work"
account: "iCloud"
source: apple-notes
---
```

## Notes and limitations

- This is macOS-only by design.
- The exporter uses AppleScript, not a private Notes database or iCloud API.
- Per-note failures are logged to stderr and do not stop the full run.
- If `osascript` or Notes is unavailable, the CLI exits with `apple-notes-md requires macOS with Notes.app available`.
- Some Notes attachment shapes still do not round-trip perfectly. `data:` images are exported, but some `cid:`-style inline attachments may currently be left as placeholders instead of extracted files.
