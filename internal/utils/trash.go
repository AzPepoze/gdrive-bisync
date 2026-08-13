package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const trashManifestName = "manifest.json"

type TrashEntry struct {
	ID           string    `json:"id"`
	OriginalPath string    `json:"originalPath"`
	TrashPath    string    `json:"trashPath"`
	DeletedAt    time.Time `json:"deletedAt"`
	IsDirectory  bool      `json:"isDirectory"`
	manifestPath string
}

func MoveToTrash(rootPath string, filePath string) error {
	_, err := TrashPath(rootPath, filePath)
	return err
}

func TrashPath(rootPath string, filePath string) (TrashEntry, error) {
	rootAbsolute, err := filepath.Abs(rootPath)
	if err != nil {
		return TrashEntry{}, err
	}
	fileAbsolute, err := filepath.Abs(filePath)
	if err != nil {
		return TrashEntry{}, err
	}
	relativePath, err := filepath.Rel(rootAbsolute, fileAbsolute)
	if err != nil || relativePath == "." || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return TrashEntry{}, fmt.Errorf("path %s is outside sync root %s", filePath, rootPath)
	}
	if strings.HasPrefix(relativePath, ".trash"+string(filepath.Separator)) || relativePath == ".trash" {
		return TrashEntry{}, fmt.Errorf("cannot trash a path already inside .trash")
	}

	info, err := os.Stat(fileAbsolute)
	if err != nil {
		return TrashEntry{}, err
	}

	deletedAt := time.Now()
	id := deletedAt.Format("20060102T150405.000000000")
	entryDirectory := filepath.Join(rootAbsolute, ".trash", deletedAt.Format("02-01-2006"), id)
	for suffix := 2; ; suffix++ {
		if _, err := os.Stat(entryDirectory); errors.Is(err, os.ErrNotExist) {
			break
		} else if err != nil {
			return TrashEntry{}, err
		}
		entryDirectory = filepath.Join(rootAbsolute, ".trash", deletedAt.Format("02-01-2006"), fmt.Sprintf("%s-%d", id, suffix))
	}

	trashPath := filepath.Join(entryDirectory, "files", relativePath)
	if err := os.MkdirAll(filepath.Dir(trashPath), 0700); err != nil {
		return TrashEntry{}, fmt.Errorf("create trash directory: %w", err)
	}
	if err := os.Rename(fileAbsolute, trashPath); err != nil {
		return TrashEntry{}, fmt.Errorf("move to trash: %w", err)
	}

	entry := TrashEntry{
		ID:           filepath.Base(entryDirectory),
		OriginalPath: filepath.ToSlash(relativePath),
		TrashPath:    trashPath,
		DeletedAt:    deletedAt,
		IsDirectory:  info.IsDir(),
	}
	if err := writeTrashManifest(filepath.Join(entryDirectory, trashManifestName), entry); err != nil {
		_ = os.Rename(trashPath, fileAbsolute)
		return TrashEntry{}, err
	}
	return entry, nil
}

func writeTrashManifest(path string, entry TrashEntry) error {
	encoded, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		return fmt.Errorf("write trash manifest: %w", err)
	}
	return nil
}

func ListTrash(rootPath string) ([]TrashEntry, error) {
	trashRoot := filepath.Join(rootPath, ".trash")
	entries := make([]TrashEntry, 0)
	manifestPaths, err := filepath.Glob(filepath.Join(trashRoot, "*", "*", trashManifestName))
	if err != nil {
		return nil, err
	}
	for _, path := range manifestPaths {
		encoded, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var entry TrashEntry
		if err := json.Unmarshal(encoded, &entry); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		entry.manifestPath = path
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].DeletedAt.After(entries[j].DeletedAt) })
	return entries, nil
}

func RestoreTrash(rootPath string, entryID string) (TrashEntry, error) {
	entries, err := ListTrash(rootPath)
	if err != nil {
		return TrashEntry{}, err
	}
	var selected *TrashEntry
	for index := range entries {
		if entries[index].ID == entryID {
			if selected != nil {
				return TrashEntry{}, fmt.Errorf("trash ID %q is ambiguous", entryID)
			}
			selected = &entries[index]
		}
	}
	if selected == nil {
		return TrashEntry{}, fmt.Errorf("trash entry %q not found", entryID)
	}

	rootAbsolute, err := filepath.Abs(rootPath)
	if err != nil {
		return TrashEntry{}, err
	}
	destination := filepath.Join(rootAbsolute, filepath.FromSlash(selected.OriginalPath))
	if !pathWithin(rootAbsolute, destination) {
		return TrashEntry{}, fmt.Errorf("trash entry contains unsafe original path %q", selected.OriginalPath)
	}
	trashRoot := filepath.Join(rootAbsolute, ".trash")
	if !pathWithin(trashRoot, selected.TrashPath) {
		return TrashEntry{}, fmt.Errorf("trash entry contains unsafe payload path")
	}
	if _, err := os.Stat(destination); err == nil {
		return TrashEntry{}, fmt.Errorf("refusing to overwrite existing path %s", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return TrashEntry{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return TrashEntry{}, err
	}
	if err := os.Rename(selected.TrashPath, destination); err != nil {
		return TrashEntry{}, fmt.Errorf("restore trash entry: %w", err)
	}
	if err := os.Remove(selected.manifestPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return TrashEntry{}, err
	}
	return *selected, nil
}

func pathWithin(root string, candidate string) bool {
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbsolute, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(rootAbsolute, candidateAbsolute)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
