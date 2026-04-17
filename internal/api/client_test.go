package api

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	"google.golang.org/api/option"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newTestDriveService(t *testing.T, transport roundTripFunc) *DriveService {
	t.Helper()

	client := &http.Client{Transport: transport}
	service, err := newDriveService(client, option.WithEndpoint("https://drive.test/"))
	if err != nil {
		t.Fatalf("failed to create test Drive service: %v", err)
	}

	return service
}

func jsonResponse(request *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}
}

func TestListFilesRecursive_UsesPaginationAndSharedDriveFlags(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []url.Values
	)

	service := newTestDriveService(t, func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		requests = append(requests, request.URL.Query())
		mu.Unlock()

		query := request.URL.Query().Get("q")
		pageToken := request.URL.Query().Get("pageToken")

		switch {
		case strings.Contains(query, "'root' in parents") && pageToken == "":
			return jsonResponse(request, http.StatusOK, `{"files":[{"id":"folder-1","name":"Folder","mimeType":"application/vnd.google-apps.folder","modifiedTime":"2026-01-01T00:00:00Z"},{"id":"file-1","name":"A.txt","mimeType":"text/plain","modifiedTime":"2026-01-01T00:00:00Z","md5Checksum":"md5-a"}],"nextPageToken":"page-2"}`), nil
		case strings.Contains(query, "'root' in parents") && pageToken == "page-2":
			return jsonResponse(request, http.StatusOK, `{"files":[{"id":"file-2","name":"B.txt","mimeType":"text/plain","modifiedTime":"2026-01-02T00:00:00Z","md5Checksum":"md5-b"}]}`), nil
		case strings.Contains(query, "'folder-1' in parents"):
			return jsonResponse(request, http.StatusOK, `{"files":[{"id":"file-3","name":"Child.txt","mimeType":"text/plain","modifiedTime":"2026-01-03T00:00:00Z","md5Checksum":"md5-c"}]}`), nil
		default:
			return jsonResponse(request, http.StatusBadRequest, `{"error":"unexpected request"}`), nil
		}
	})

	files, err := service.ListFilesRecursive(context.Background(), ListFilesRequest{
		FolderID:      "root",
		MaxConcurrent: 1,
		MaxRetries:    1,
	})
	if err != nil {
		t.Fatalf("list files failed: %v", err)
	}

	expectedPaths := []string{"Folder", "Folder/Child.txt", "A.txt", "B.txt"}
	for _, path := range expectedPaths {
		if _, ok := files[path]; !ok {
			t.Fatalf("expected remote path %s to be listed", path)
		}
	}

	if len(requests) != 3 {
		t.Fatalf("expected 3 list requests, got %d", len(requests))
	}
	for _, query := range requests {
		if query.Get("supportsAllDrives") != "true" {
			t.Fatalf("expected supportsAllDrives=true, got %q", query.Get("supportsAllDrives"))
		}
		if query.Get("includeItemsFromAllDrives") != "true" {
			t.Fatalf("expected includeItemsFromAllDrives=true, got %q", query.Get("includeItemsFromAllDrives"))
		}
	}
}

func TestFetchChanges_UsesSharedDriveFlagsAndTracksNewStartPageToken(t *testing.T) {
	var (
		mu       sync.Mutex
		requests []url.Values
	)

	service := newTestDriveService(t, func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		requests = append(requests, request.URL.Query())
		mu.Unlock()

		switch request.URL.Query().Get("pageToken") {
		case "start-token":
			return jsonResponse(request, http.StatusOK, `{"changes":[{"fileId":"file-1","removed":false,"file":{"id":"file-1","name":"A.txt","parents":["root"],"mimeType":"text/plain","modifiedTime":"2026-01-01T00:00:00Z","md5Checksum":"md5-a"}}],"nextPageToken":"page-2"}`), nil
		case "page-2":
			return jsonResponse(request, http.StatusOK, `{"changes":[{"fileId":"file-2","removed":true}],"newStartPageToken":"new-start"}`), nil
		default:
			return jsonResponse(request, http.StatusBadRequest, `{"error":"unexpected page token"}`), nil
		}
	})

	result, err := service.FetchChanges(context.Background(), FetchChangesRequest{
		PageToken: "start-token",
		PageSize:  1000,
	})
	if err != nil {
		t.Fatalf("fetch changes failed: %v", err)
	}

	if len(result.Changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(result.Changes))
	}
	if result.NewPageToken != "new-start" {
		t.Fatalf("expected new start token to be preserved, got %q", result.NewPageToken)
	}

	if len(requests) != 2 {
		t.Fatalf("expected 2 change requests, got %d", len(requests))
	}
	for _, query := range requests {
		if query.Get("supportsAllDrives") != "true" {
			t.Fatalf("expected supportsAllDrives=true, got %q", query.Get("supportsAllDrives"))
		}
		if query.Get("includeItemsFromAllDrives") != "true" {
			t.Fatalf("expected includeItemsFromAllDrives=true, got %q", query.Get("includeItemsFromAllDrives"))
		}
	}
}

func TestGetStartPageToken_RetriesTransientFailures(t *testing.T) {
	var attempts int

	service := newTestDriveService(t, func(request *http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return jsonResponse(request, http.StatusInternalServerError, `{"error":"temporary failure"}`), nil
		}

		return jsonResponse(request, http.StatusOK, `{"startPageToken":"stable-token"}`), nil
	})

	token, err := service.GetStartPageToken(context.Background())
	if err != nil {
		t.Fatalf("get start page token failed: %v", err)
	}

	if token != "stable-token" {
		t.Fatalf("expected stable-token, got %q", token)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts after transient failure, got %d", attempts)
	}
}
