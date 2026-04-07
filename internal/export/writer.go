package export

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charliewilco/AppleNotesExporter/internal/convert"
	"github.com/charliewilco/AppleNotesExporter/internal/notes"
)

type Store interface {
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(path string, data []byte, perm os.FileMode) error
	Stat(path string) (fs.FileInfo, error)
}

type Options struct {
	OutputDir string
	Flat      bool
	Overwrite bool
	NoImages  bool
	DryRun    bool
	Workers   int
}

type Result struct {
	NotePath   string
	Skipped    bool
	AssetCount int
}

type Writer struct {
	store      Store
	options    Options
	httpClient *http.Client
	mu         sync.Mutex
	reserved   map[string]string
}

type osStore struct{}

type dryRunStore struct{}

func NewWriter(store Store, options Options) *Writer {
	workers := options.Workers
	if workers < 1 {
		workers = 1
	}
	options.Workers = workers

	return &Writer{
		store:   store,
		options: options,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		reserved: make(map[string]string),
	}
}

func NewOSStore() Store {
	return osStore{}
}

func NewDryRunStore() Store {
	return dryRunStore{}
}

func (writer *Writer) Export(ctx context.Context, note notes.Note) (Result, error) {
	notePath, err := writer.planPath(note)
	if err != nil {
		return Result{}, err
	}

	if !writer.options.Overwrite {
		if _, err := writer.store.Stat(notePath); err == nil {
			return Result{NotePath: notePath, Skipped: true}, nil
		} else if !os.IsNotExist(err) {
			return Result{}, fmt.Errorf("check existing note file %s: %w", notePath, err)
		}
	}

	baseName := strings.TrimSuffix(filepath.Base(notePath), filepath.Ext(notePath))
	converted, err := convert.HTMLToMarkdown(note.BodyHTML, convert.Options{
		AssetDir:    "assets",
		AssetPrefix: baseName,
		NoImages:    writer.options.NoImages,
	})
	if err != nil {
		return Result{NotePath: notePath}, fmt.Errorf("convert HTML for %q: %w", note.EffectiveTitle(), err)
	}

	noteDir := filepath.Dir(notePath)
	if err := writer.writeAssets(ctx, noteDir, converted.Assets); err != nil {
		return Result{NotePath: notePath}, err
	}

	if err := writer.store.MkdirAll(noteDir, 0o755); err != nil {
		return Result{NotePath: notePath}, fmt.Errorf("create note directory %s: %w", noteDir, err)
	}

	document := buildDocument(note, converted.Markdown)
	if err := writer.store.WriteFile(notePath, []byte(document), 0o644); err != nil {
		return Result{NotePath: notePath}, fmt.Errorf("write note file %s: %w", notePath, err)
	}

	return Result{
		NotePath:   notePath,
		AssetCount: len(converted.Assets),
	}, nil
}

func (writer *Writer) planPath(note notes.Note) (string, error) {
	targetDir := writer.noteDirectory(note)
	baseName := sanitizeFileName(note.EffectiveTitle())
	if baseName == "" {
		baseName = "Untitled Note"
	}

	candidate := filepath.Join(targetDir, baseName+".md")
	collisionSuffix := " " + note.ShortID()

	writer.mu.Lock()
	defer writer.mu.Unlock()

	if ownerID, exists := writer.reserved[candidate]; exists && ownerID != note.ID {
		baseName = truncateWithSuffix(baseName, collisionSuffix, 80)
		candidate = filepath.Join(targetDir, baseName+collisionSuffix+".md")
	}

	writer.reserved[candidate] = note.ID

	return candidate, nil
}

func (writer *Writer) noteDirectory(note notes.Note) string {
	if writer.options.Flat {
		return writer.options.OutputDir
	}

	accountName := sanitizePathSegment(note.Account)
	if accountName == "" {
		accountName = "Unknown Account"
	}

	parts := []string{writer.options.OutputDir, accountName}
	folderPath := strings.TrimSpace(note.FolderPath)
	if folderPath == "" {
		folderPath = note.Folder
	}
	if folderPath == "" {
		folderPath = "Notes"
	}

	for _, segment := range strings.Split(folderPath, "/") {
		cleaned := sanitizePathSegment(segment)
		if cleaned == "" {
			continue
		}
		parts = append(parts, cleaned)
	}

	return filepath.Join(parts...)
}

func (writer *Writer) writeAssets(ctx context.Context, noteDir string, assets []convert.Asset) error {
	if len(assets) == 0 {
		return nil
	}

	assetDir := filepath.Join(noteDir, "assets")
	if err := writer.store.MkdirAll(assetDir, 0o755); err != nil {
		return fmt.Errorf("create asset directory %s: %w", assetDir, err)
	}

	jobChannel := make(chan convert.Asset)
	errorChannel := make(chan error, len(assets))
	workerCount := writer.options.Workers
	if workerCount > len(assets) {
		workerCount = len(assets)
	}

	var waitGroup sync.WaitGroup
	for workerIndex := 0; workerIndex < workerCount; workerIndex++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for asset := range jobChannel {
				if err := writer.writeAsset(ctx, noteDir, asset); err != nil {
					errorChannel <- err
				}
			}
		}()
	}

	for _, asset := range assets {
		jobChannel <- asset
	}

	close(jobChannel)
	waitGroup.Wait()
	close(errorChannel)

	var errors []string
	for err := range errorChannel {
		errors = append(errors, err.Error())
	}

	if len(errors) > 0 {
		return fmt.Errorf("write note assets: %s", strings.Join(errors, "; "))
	}

	return nil
}

func (writer *Writer) writeAsset(ctx context.Context, noteDir string, asset convert.Asset) error {
	var (
		payload []byte
		err     error
	)

	switch asset.SourceType {
	case convert.AssetSourceDataURL:
		_, payload, err = decodeDataURL(asset.Source)
	case convert.AssetSourceRemote:
		payload, err = writer.downloadAsset(ctx, asset.Source)
	default:
		err = fmt.Errorf("unsupported asset source type %q", asset.SourceType)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", asset.RelativePath, err)
	}

	assetPath := filepath.Join(noteDir, filepath.FromSlash(asset.RelativePath))
	assetDir := filepath.Dir(assetPath)
	if err := writer.store.MkdirAll(assetDir, 0o755); err != nil {
		return fmt.Errorf("create asset path %s: %w", assetDir, err)
	}

	if err := writer.store.WriteFile(assetPath, payload, 0o644); err != nil {
		return fmt.Errorf("write asset %s: %w", assetPath, err)
	}

	return nil
}

func (writer *Writer) downloadAsset(ctx context.Context, source string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}

	response, err := writer.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected HTTP status %s", response.Status)
	}

	return io.ReadAll(response.Body)
}

func (osStore) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func (osStore) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (osStore) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (dryRunStore) MkdirAll(path string, perm os.FileMode) error {
	return nil
}

func (dryRunStore) WriteFile(path string, data []byte, perm os.FileMode) error {
	return nil
}

func (dryRunStore) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func buildDocument(note notes.Note, markdown string) string {
	title := escapeYAMLString(note.EffectiveTitle())
	folder := note.Folder
	if folder == "" {
		folder = note.FolderPath
	}

	frontmatter := fmt.Sprintf(
		"---\n"+
			"title: \"%s\"\n"+
			"created: %s\n"+
			"modified: %s\n"+
			"folder: \"%s\"\n"+
			"account: \"%s\"\n"+
			"source: apple-notes\n"+
			"---\n\n",
		title,
		note.Created.UTC().Format(time.RFC3339),
		note.Modified.UTC().Format(time.RFC3339),
		escapeYAMLString(folder),
		escapeYAMLString(note.Account),
	)

	body := strings.TrimLeft(markdown, "\n")
	if strings.TrimSpace(body) == "" {
		body = "_Empty note._\n"
	}

	return frontmatter + body
}

func sanitizeFileName(value string) string {
	value = truncateRunes(sanitizePathSegment(value), 80)
	return strings.Trim(value, " .")
}

func sanitizePathSegment(value string) string {
	var builder strings.Builder
	for _, character := range strings.TrimSpace(value) {
		switch character {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|':
			builder.WriteRune('-')
		default:
			if character < 32 {
				continue
			}
			builder.WriteRune(character)
		}
	}

	cleaned := strings.TrimSpace(builder.String())
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	cleaned = strings.Trim(cleaned, ".")
	return cleaned
}

func truncateWithSuffix(value string, suffix string, limit int) string {
	if suffix == "" {
		return truncateRunes(value, limit)
	}

	baseLimit := limit - len([]rune(suffix))
	if baseLimit < 1 {
		return ""
	}

	return truncateRunes(value, baseLimit)
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	return string(runes[:limit])
}

func escapeYAMLString(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func decodeDataURL(raw string) (string, []byte, error) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "data:") {
		return "", nil, fmt.Errorf("invalid data URL")
	}

	metadata, encodedData, found := strings.Cut(strings.TrimPrefix(raw, "data:"), ",")
	if !found {
		return "", nil, fmt.Errorf("invalid data URL payload")
	}

	mediaType := "text/plain"
	isBase64 := false
	if metadata != "" {
		parsedMediaType, parameters, err := mime.ParseMediaType(metadata)
		if err == nil && parsedMediaType != "" {
			mediaType = parsedMediaType
			if _, ok := parameters["base64"]; ok {
				isBase64 = true
			}
		} else {
			parts := strings.Split(metadata, ";")
			if len(parts) > 0 && parts[0] != "" {
				mediaType = parts[0]
			}
			for _, part := range parts[1:] {
				if strings.EqualFold(strings.TrimSpace(part), "base64") {
					isBase64 = true
				}
			}
		}
	}

	if isBase64 {
		data, err := base64.StdEncoding.DecodeString(encodedData)
		if err != nil {
			return "", nil, err
		}
		return mediaType, data, nil
	}

	decoded, err := url.PathUnescape(encodedData)
	if err != nil {
		return "", nil, err
	}

	return mediaType, []byte(decoded), nil
}
