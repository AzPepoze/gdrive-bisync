package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"gdrive-bisync/internal/services/logger"
	"gdrive-bisync/internal/types"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type DriveService struct {
	srv *drive.Service
}

func NewDriveService(client *http.Client) (*DriveService, error) {
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, err
	}
	return &DriveService{srv: srv}, nil
}

func (s *DriveService) ListFilesRecursive(folderId string, ignoreRegexps []*regexp.Regexp, onProgress func(path string), maxConcurrent int, maxRetries int) (types.DriveFileMap, error) {
	fileMap := make(types.DriveFileMap)
	var mu sync.Mutex
	var wg sync.WaitGroup

	if maxConcurrent <= 0 {
		maxConcurrent = 20
	}
	if maxRetries <= 0 {
		maxRetries = 10
	}

	sem := make(chan struct{}, maxConcurrent)
	errChan := make(chan error, 1)

	wg.Add(1)
	go s.traverseParallel(folderId, "", ignoreRegexps, fileMap, onProgress, &wg, &mu, sem, errChan, maxRetries)

	wg.Wait()
	close(errChan)

	if err := <-errChan; err != nil {
		return nil, err
	}

	return fileMap, nil
}

func (s *DriveService) traverseParallel(
	currentFolderId string,
	currentPath string,
	ignoreRegexps []*regexp.Regexp,
	fileMap types.DriveFileMap,
	onProgress func(path string),
	wg *sync.WaitGroup,
	mu *sync.Mutex,
	sem chan struct{},
	errChan chan error,
	maxRetries int,
) {
	defer wg.Done()

	select {
	case <-errChan:
		return
	default:
	}

	if onProgress != nil {
		onProgress(currentPath)
	}

	retryOp := func(op func() error) error {
		for i := 0; i < maxRetries; i++ {
			err := op()
			if err == nil {
				return nil
			}
			logger.Debug("Retrying API call", "attempt", i+1, "error", err)
			if i < maxRetries-1 {
				time.Sleep(1 * time.Second)
			} else {
				return err
			}
		}
		return nil
	}

	sem <- struct{}{}
	defer func() { <-sem }()

	pageToken := ""
	var folderIds []struct {
		id   string
		path string
	}

	for {
		var res *drive.FileList
		err := retryOp(func() error {
			q := fmt.Sprintf("'%s' in parents and trashed = false", currentFolderId)
			call := s.srv.Files.List().Q(q).
				Fields("nextPageToken, files(id, name, mimeType, modifiedTime, md5Checksum)").
				PageSize(1000)

			if pageToken != "" {
				call.PageToken(pageToken)
			}

			r, e := call.Do()
			if e != nil {
				return e
			}
			res = r
			return nil
		})

		if err != nil {
			select {
			case errChan <- fmt.Errorf("list files failed for %s: %w", currentPath, err):
			default:
			}
			return
		}

		mu.Lock()
		for _, file := range res.Files {
			filePath := filepath.Join(currentPath, file.Name)
			isDirectory := file.MimeType == "application/vnd.google-apps.folder"

			ignored := false
			for _, re := range ignoreRegexps {
				if re.MatchString(filePath) {
					ignored = true
					break
				}
			}
			if ignored {
				logger.Debug("Ignoring Drive file/folder", "path", filePath)
				continue
			}

			modTime, err := time.Parse(time.RFC3339, file.ModifiedTime)
			if err != nil {
				logger.Warn("Error parsing modified time", "file", file.Name, "error", err)
				modTime = time.Now()
			}

			existing, exists := fileMap[filePath]
			if exists && !isDirectory {
				if modTime.Before(existing.ModifiedTime) || modTime.Equal(existing.ModifiedTime) {
					continue
				}
			}

			fileMap[filePath] = &types.DriveFile{
				ID:           file.Id,
				Name:         file.Name,
				Path:         filePath,
				ModifiedTime: modTime,
				MD5Checksum:  file.Md5Checksum,
				IsDirectory:  isDirectory,
			}

			if isDirectory {
				folderIds = append(folderIds, struct{ id, path string }{file.Id, filePath})
			}
		}
		mu.Unlock()

		pageToken = res.NextPageToken
		if pageToken == "" {
			break
		}
	}

	for _, f := range folderIds {
		wg.Add(1)
		go s.traverseParallel(f.id, f.path, ignoreRegexps, fileMap, onProgress, wg, mu, sem, errChan, maxRetries)
	}
}

func (s *DriveService) GetStartPageToken() (string, error) {
	res, err := s.srv.Changes.GetStartPageToken().Do()
	if err != nil {
		return "", err
	}
	return res.StartPageToken, nil
}

func (s *DriveService) FetchChanges(pageToken string) ([]*drive.Change, string, error) {
	var changes []*drive.Change
	for {
		call := s.srv.Changes.List(pageToken).
			Fields("nextPageToken, newStartPageToken, changes(fileId, removed, file(id, name, parents, mimeType, modifiedTime, md5Checksum, trashed))").
			PageSize(1000)

		res, err := call.Do()
		if err != nil {
			return nil, pageToken, err
		}

		changes = append(changes, res.Changes...)

		if res.NewStartPageToken != "" {
			return changes, res.NewStartPageToken, nil
		}

		pageToken = res.NextPageToken
		if pageToken == "" {
			break
		}
	}
	return changes, pageToken, nil
}

// DownloadFile downloads a file from Drive.
func (s *DriveService) DownloadFile(fileId string, destinationPath string) error {
	res, err := s.srv.Files.Get(fileId).Download()
	if err != nil {
		return err
	}
	defer res.Body.Close()

	out, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, res.Body)
	return err
}

// UploadOrUpdateFile uploads or updates a file.
func (s *DriveService) UploadOrUpdateFile(localPath string, remoteInfo struct {
	Name     string
	FolderID string
	FileID   string
}) (*drive.File, error) {
	f, err := os.Open(localPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	if remoteInfo.FileID != "" {
		file := &drive.File{}
		return s.srv.Files.Update(remoteInfo.FileID, file).Media(f).Fields("id, name, modifiedTime, md5Checksum").Do()
	}

	file := &drive.File{
		Name:    remoteInfo.Name,
		Parents: []string{remoteInfo.FolderID},
	}
	return s.srv.Files.Create(file).Media(f).Fields("id, name, modifiedTime, md5Checksum").Do()
}

// CreateFolder creates a folder.
func (s *DriveService) CreateFolder(parentFolderId string, folderName string) (string, error) {
	file := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentFolderId},
	}
	res, err := s.srv.Files.Create(file).Fields("id").Do()
	if err != nil {
		return "", err
	}
	return res.Id, nil
}

// DeleteFilePermanently deletes a file.
func (s *DriveService) DeleteFilePermanently(fileId string) error {
	return s.srv.Files.Delete(fileId).Do()
}

// TrashRemoteFile trashes a file.
func (s *DriveService) TrashRemoteFile(fileId string) error {
	file := &drive.File{Trashed: true}
	_, err := s.srv.Files.Update(fileId, file).Do()
	return err
}
