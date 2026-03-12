package core

import "gdrive-bisync/internal/types"

type State struct {
	PageToken   string
	RemoteFiles types.DriveFileMap
}
