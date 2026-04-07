package export

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charliewilco/AppleNotesExporter/internal/notes"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP+Z7S3GQAAAABJRU5ErkJggg=="

func TestWriterPlanPathAddsShortIDOnCollision(t *testing.T) {
	t.Parallel()

	writer := NewWriter(NewDryRunStore(), Options{
		OutputDir: t.TempDir(),
	})

	first := notes.Note{
		ID:      "note-1",
		Title:   "Duplicate",
		Account: "iCloud",
		Folder:  "Work",
	}
	second := notes.Note{
		ID:      "note-2",
		Title:   "Duplicate",
		Account: "iCloud",
		Folder:  "Work",
	}

	firstPath, err := writer.planPath(first)
	if err != nil {
		t.Fatalf("planPath(first) returned error: %v", err)
	}

	secondPath, err := writer.planPath(second)
	if err != nil {
		t.Fatalf("planPath(second) returned error: %v", err)
	}

	if !strings.HasSuffix(firstPath, filepath.Join("iCloud", "Work", "Duplicate.md")) {
		t.Fatalf("unexpected first path %q", firstPath)
	}

	expectedSecondSuffix := filepath.Join("iCloud", "Work", "Duplicate-"+second.ShortID()+".md")
	if !strings.HasSuffix(secondPath, expectedSecondSuffix) {
		t.Fatalf("unexpected second path %q", secondPath)
	}
}

func TestWriterExportWritesFrontmatterAndAssets(t *testing.T) {
	t.Parallel()

	outputDir := t.TempDir()
	writer := NewWriter(NewOSStore(), Options{
		OutputDir: outputDir,
		Workers:   2,
	})

	created := time.Date(2024, time.March, 15, 10, 30, 0, 0, time.UTC)
	modified := time.Date(2024, time.November, 2, 14, 22, 0, 0, time.UTC)
	note := notes.Note{
		ID:       "test-note",
		Title:    "Project Snapshot",
		BodyHTML: `<div>Hello <b>team</b></div><div><img src="data:image/png;base64,` + onePixelPNG + `" alt="Preview"></div>`,
		Created:  created,
		Modified: modified,
		Folder:   "Work",
		Account:  "iCloud",
	}

	result, err := writer.Export(context.Background(), note)
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}

	if result.Skipped {
		t.Fatal("expected note to be written, but it was skipped")
	}

	documentBytes, err := os.ReadFile(result.NotePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) returned error: %v", result.NotePath, err)
	}

	document := string(documentBytes)
	for _, snippet := range []string{
		`title: "Project Snapshot"`,
		`created: 2024-03-15T10:30:00Z`,
		`modified: 2024-11-02T14:22:00Z`,
		`folder: "Work"`,
		`account: "iCloud"`,
		`Hello **team**`,
		`![Preview](assets/Project Snapshot-001.png)`,
	} {
		if !strings.Contains(document, snippet) {
			t.Fatalf("document missing snippet %q:\n%s", snippet, document)
		}
	}

	assetPath := filepath.Join(filepath.Dir(result.NotePath), "assets", "Project Snapshot-001.png")
	if _, err := os.Stat(assetPath); err != nil {
		t.Fatalf("expected asset file %q to exist: %v", assetPath, err)
	}
}
