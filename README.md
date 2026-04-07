# apple-notes-md

`apple-notes-md` is a macOS-only CLI for exporting Apple Notes to Markdown files.
It uses `osascript` and AppleScript to read note data, then writes one `.md` file per note with a folder structure that mirrors Notes accounts and folders.

## Requirements

- macOS with Notes.app available
- Go 1.23 or newer

## Install

From a local checkout:

```bash
go install .
```

If you want to build and install from this repository with `just`:

```bash
just build
just install
```

For linting:

```bash
just lint
```

## Usage

```bash
apple-notes-md export [flags]
```

### Flags

- `-o, --output string` Output directory. Default: `~/Downloads/AppleNotesExport`
- `-f, --folder string` Export only notes in this folder, exact match
- `-q, --query string` Filter notes by title substring
- `--since string` Only export notes modified after this date, RFC3339 or `YYYY-MM-DD`
- `--flat` Write all files into a single directory
- `--overwrite` Overwrite existing files instead of skipping them
- `--no-images` Skip image downloading
- `--dry-run` Print what would be exported without writing files
- `-v, --verbose` Verbose logging
- `--workers int` Number of concurrent image download workers. Default: `4`

## Examples

Export everything to the default directory:

```bash
apple-notes-md export
```

Export only notes from a folder into a custom location:

```bash
apple-notes-md export --folder Work --output ~/Desktop/NotesExport
```

Preview an export without writing files:

```bash
apple-notes-md export --query "project" --since 2024-03-15 --dry-run
```

Export into a flat directory and overwrite existing files:

```bash
apple-notes-md export --flat --overwrite
```

## Behavior

- Each note becomes a Markdown file with YAML frontmatter.
- Folder and account names are preserved in the output path unless `--flat` is set.
- Existing files are skipped by default.
- Per-note errors are reported to stderr and do not stop the full export.
- If Notes cannot be accessed, the CLI should exit with a clear macOS-specific error message.
