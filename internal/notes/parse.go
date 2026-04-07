package notes

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Note struct {
	ID         string
	Title      string
	BodyHTML   string
	Created    time.Time
	Modified   time.Time
	Folder     string
	FolderPath string
	Account    string
	FetchError string
}

type FilterOptions struct {
	Folder string
	Query  string
	Since  *time.Time
}

type payloadNote struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Created    string `json:"created"`
	Modified   string `json:"modified"`
	Folder     string `json:"folder"`
	FolderPath string `json:"folder_path"`
	Account    string `json:"account"`
	FetchError string `json:"fetch_error"`
}

func ParsePayload(raw []byte) ([]Note, error) {
	var payload []payloadNote
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode Apple Notes payload: %w", err)
	}

	notes := make([]Note, 0, len(payload))
	for _, item := range payload {
		created, err := parseTimestamp(item.Created)
		if err != nil {
			return nil, fmt.Errorf("parse creation date for note %q: %w", item.ID, err)
		}

		modified, err := parseTimestamp(item.Modified)
		if err != nil {
			return nil, fmt.Errorf("parse modification date for note %q: %w", item.ID, err)
		}

		note := Note{
			ID:         strings.TrimSpace(item.ID),
			Title:      strings.TrimSpace(item.Title),
			BodyHTML:   item.Body,
			Created:    created,
			Modified:   modified,
			Folder:     strings.TrimSpace(item.Folder),
			FolderPath: strings.TrimSpace(item.FolderPath),
			Account:    strings.TrimSpace(item.Account),
			FetchError: strings.TrimSpace(item.FetchError),
		}

		if note.FolderPath == "" {
			note.FolderPath = note.Folder
		}

		notes = append(notes, note)
	}

	return notes, nil
}

func (options FilterOptions) Match(note Note) bool {
	if options.Folder != "" {
		if !strings.EqualFold(note.Folder, options.Folder) && !strings.EqualFold(note.FolderPath, options.Folder) {
			return false
		}
	}

	if options.Query != "" && !strings.Contains(strings.ToLower(note.EffectiveTitle()), strings.ToLower(options.Query)) {
		return false
	}

	if options.Since != nil && !note.Modified.After(*options.Since) {
		return false
	}

	return true
}

func (note Note) EffectiveTitle() string {
	title := strings.TrimSpace(note.Title)
	if title != "" {
		return title
	}

	return "Untitled Note"
}

func (note Note) ShortID() string {
	sum := sha1.Sum([]byte(note.ID))
	return hex.EncodeToString(sum[:])[:8]
}

func parseTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}

	return parsed, nil
}
