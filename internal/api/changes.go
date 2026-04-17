package api

import (
	"context"
	"fmt"

	"google.golang.org/api/drive/v3"
)

func (s *DriveService) GetStartPageToken(ctx context.Context) (string, error) {
	call := s.srv.Changes.GetStartPageToken().SupportsAllDrives(true).Context(ctx)

	var response *drive.StartPageToken
	err := s.retry(ctx, "get start page token", normalizeRetryConfig(3), func() error {
		var callErr error
		response, callErr = call.Do()
		return callErr
	})
	if err != nil {
		return "", err
	}

	return response.StartPageToken, nil
}

func (s *DriveService) FetchChanges(ctx context.Context, request FetchChangesRequest) (*FetchChangesResult, error) {
	request = normalizeFetchChangesRequest(request)

	result := &FetchChangesResult{
		Changes:      make([]*drive.Change, 0),
		NewPageToken: request.PageToken,
	}

	pageToken := request.PageToken
	for pageToken != "" {
		var response *drive.ChangeList
		operation := fmt.Sprintf("fetch changes for token %s", pageToken)
		err := s.retry(ctx, operation, normalizeRetryConfig(3), func() error {
			call := applySharedDriveOptionsToChangesList(s.srv.Changes.List(pageToken)).
				Fields(driveChangeFieldsSelector()).
				PageSize(request.PageSize).
				Context(ctx)

			var callErr error
			response, callErr = call.Do()
			return callErr
		})
		if err != nil {
			return nil, err
		}

		result.Changes = append(result.Changes, response.Changes...)
		if response.NewStartPageToken != "" {
			result.NewPageToken = response.NewStartPageToken
			break
		}

		pageToken = response.NextPageToken
		if pageToken == "" {
			break
		}
		result.NewPageToken = pageToken
	}

	return result, nil
}
