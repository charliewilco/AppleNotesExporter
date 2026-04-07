package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/charliewilco/AppleNotesExporter/internal/export"
	"github.com/charliewilco/AppleNotesExporter/internal/notes"
)

type exportFlags struct {
	output    string
	folder    string
	query     string
	since     string
	flat      bool
	overwrite bool
	noImages  bool
	dryRun    bool
	verbose   bool
	workers   int
}

func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "apple-notes-md",
		Short:         "Export Apple Notes to Markdown",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	root.AddCommand(newExportCommand())

	return root
}

func newExportCommand() *cobra.Command {
	flags := exportFlags{
		output:  defaultOutputDir(),
		workers: 4,
	}

	command := &cobra.Command{
		Use:   "export",
		Short: "Export Apple Notes to Markdown files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd.Context(), cmd, flags)
		},
	}

	command.Flags().StringVarP(&flags.output, "output", "o", flags.output, "Output directory")
	command.Flags().StringVarP(&flags.folder, "folder", "f", "", "Export only notes in this folder (exact match)")
	command.Flags().StringVarP(&flags.query, "query", "q", "", "Filter notes by title substring")
	command.Flags().StringVar(&flags.since, "since", "", "Only export notes modified after this date (RFC3339 or YYYY-MM-DD)")
	command.Flags().BoolVar(&flags.flat, "flat", false, "Write all files into a single flat directory")
	command.Flags().BoolVar(&flags.overwrite, "overwrite", false, "Overwrite existing files")
	command.Flags().BoolVar(&flags.noImages, "no-images", false, "Skip image downloading")
	command.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Print what would be exported without writing files")
	command.Flags().BoolVarP(&flags.verbose, "verbose", "v", false, "Verbose logging")
	command.Flags().IntVar(&flags.workers, "workers", flags.workers, "Number of concurrent image download workers")

	return command
}

func runExport(ctx context.Context, cmd *cobra.Command, flags exportFlags) error {
	if flags.workers < 1 {
		return errors.New("--workers must be at least 1")
	}

	since, err := parseSince(flags.since)
	if err != nil {
		return err
	}

	outputDir, err := expandHome(flags.output)
	if err != nil {
		return err
	}

	noteList, err := notes.Fetch(ctx)
	if err != nil {
		return err
	}

	store := export.NewOSStore()
	if flags.dryRun {
		store = export.NewDryRunStore()
	}

	writer := export.NewWriter(store, export.Options{
		OutputDir: outputDir,
		Flat:      flags.flat,
		Overwrite: flags.overwrite,
		NoImages:  flags.noImages,
		DryRun:    flags.dryRun,
		Workers:   flags.workers,
	})

	filters := notes.FilterOptions{
		Folder: flags.folder,
		Query:  flags.query,
		Since:  since,
	}

	stdout := cmd.OutOrStdout()
	stderr := cmd.ErrOrStderr()

	exportedCount := 0
	errorCount := 0

	for _, note := range noteList {
		if note.FetchError != "" {
			errorCount++
			fmt.Fprintf(stderr, "error fetching note %q (%s): %s\n", note.EffectiveTitle(), note.ID, note.FetchError)
			continue
		}

		if !filters.Match(note) {
			continue
		}

		result, exportErr := writer.Export(ctx, note)
		if exportErr != nil {
			errorCount++
			fmt.Fprintf(stderr, "error exporting note %q (%s): %v\n", note.EffectiveTitle(), note.ID, exportErr)
			continue
		}

		if result.Skipped {
			if flags.verbose || flags.dryRun {
				fmt.Fprintf(stderr, "skipped existing %s\n", result.NotePath)
			}
			continue
		}

		exportedCount++

		switch {
		case flags.dryRun:
			fmt.Fprintf(stdout, "Would export %s -> %s\n", note.EffectiveTitle(), result.NotePath)
		case flags.verbose:
			fmt.Fprintf(stderr, "exported %s -> %s\n", note.EffectiveTitle(), result.NotePath)
		}
	}

	if flags.dryRun {
		fmt.Fprintf(stdout, "Would export %d notes. %d errors.\n", exportedCount, errorCount)
		return nil
	}

	if errorCount > 0 {
		fmt.Fprintf(stdout, "Exported %d notes. %d errors. See stderr for details.\n", exportedCount, errorCount)
		return nil
	}

	fmt.Fprintf(stdout, "Exported %d notes. %d errors.\n", exportedCount, errorCount)
	return nil
}

func defaultOutputDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "./AppleNotesExport"
	}

	return filepath.Join(home, "Downloads", "AppleNotesExport")
}

func expandHome(pathValue string) (string, error) {
	if pathValue == "" {
		return "", errors.New("output directory cannot be empty")
	}

	if pathValue == "~" {
		return os.UserHomeDir()
	}

	if strings.HasPrefix(pathValue, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}

		pathValue = filepath.Join(home, strings.TrimPrefix(pathValue, "~/"))
	}

	return filepath.Clean(pathValue), nil
}

func parseSince(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return &parsed, nil
	}

	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		return nil, fmt.Errorf("parse --since value %q: expected RFC3339 or YYYY-MM-DD", value)
	}

	return &parsed, nil
}
