package store

import (
	"encoding/json"
	"time"

	"go.etcd.io/bbolt"

	"gdrive-bisync/internal/types"
)

var (
	bucketRemoteFiles = []byte("remoteFiles")
	bucketMetadata    = []byte("metadata")
	bucketConfig      = []byte("config")
	keyPageToken      = []byte("pageToken")
)

type Store struct {
	database *bbolt.DB
}

func Open(path string) (*Store, error) {
	database, err := bbolt.Open(path, 0644, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}

	err = database.Update(func(transaction *bbolt.Tx) error {
		for _, bucket := range [][]byte{bucketRemoteFiles, bucketMetadata, bucketConfig} {
			if _, err := transaction.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		database.Close()
		return nil, err
	}

	return &Store{database: database}, nil
}

func (store *Store) Close() error {
	return store.database.Close()
}

func (store *Store) LoadRemoteFiles() (types.DriveFileMap, error) {
	remoteFiles := make(types.DriveFileMap)

	err := store.database.View(func(transaction *bbolt.Tx) error {
		bucket := transaction.Bucket(bucketRemoteFiles)
		return bucket.ForEach(func(key, value []byte) error {
			var driveFile types.DriveFile
			if err := json.Unmarshal(value, &driveFile); err != nil {
				return err
			}
			remoteFiles[string(key)] = &driveFile
			return nil
		})
	})

	return remoteFiles, err
}

func (store *Store) LoadMetadata() (map[string]*types.FileMetadata, error) {
	metadata := make(map[string]*types.FileMetadata)

	err := store.database.View(func(transaction *bbolt.Tx) error {
		bucket := transaction.Bucket(bucketMetadata)
		return bucket.ForEach(func(key, value []byte) error {
			var fileMetadata types.FileMetadata
			if err := json.Unmarshal(value, &fileMetadata); err != nil {
				return err
			}
			metadata[string(key)] = &fileMetadata
			return nil
		})
	})

	return metadata, err
}

func (store *Store) SaveRemoteFiles(changedFiles types.DriveFileMap, deletedPaths []string) error {
	if len(changedFiles) == 0 && len(deletedPaths) == 0 {
		return nil
	}
	return store.database.Update(func(transaction *bbolt.Tx) error {
		bucket := transaction.Bucket(bucketRemoteFiles)

		for path, driveFile := range changedFiles {
			encoded, err := json.Marshal(driveFile)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(path), encoded); err != nil {
				return err
			}
		}

		for _, path := range deletedPaths {
			if err := bucket.Delete([]byte(path)); err != nil {
				return err
			}
		}

		return nil
	})
}

func (store *Store) ReplaceAllRemoteFiles(remoteFiles types.DriveFileMap) error {
	return store.database.Update(func(transaction *bbolt.Tx) error {
		if err := transaction.DeleteBucket(bucketRemoteFiles); err != nil && err != bbolt.ErrBucketNotFound {
			return err
		}
		bucket, err := transaction.CreateBucket(bucketRemoteFiles)
		if err != nil {
			return err
		}
		for path, driveFile := range remoteFiles {
			encoded, err := json.Marshal(driveFile)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(path), encoded); err != nil {
				return err
			}
		}
		return nil
	})
}

func (store *Store) SaveMetadata(changedMetadata map[string]*types.FileMetadata, deletedPaths []string) error {
	if len(changedMetadata) == 0 && len(deletedPaths) == 0 {
		return nil
	}
	return store.database.Update(func(transaction *bbolt.Tx) error {
		bucket := transaction.Bucket(bucketMetadata)

		for path, fileMetadata := range changedMetadata {
			encoded, err := json.Marshal(fileMetadata)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(path), encoded); err != nil {
				return err
			}
		}

		for _, path := range deletedPaths {
			if err := bucket.Delete([]byte(path)); err != nil {
				return err
			}
		}

		return nil
	})
}

func (store *Store) ReplaceAllMetadata(metadata map[string]*types.FileMetadata) error {
	return store.database.Update(func(transaction *bbolt.Tx) error {
		if err := transaction.DeleteBucket(bucketMetadata); err != nil && err != bbolt.ErrBucketNotFound {
			return err
		}
		bucket, err := transaction.CreateBucket(bucketMetadata)
		if err != nil {
			return err
		}
		for path, fileMetadata := range metadata {
			encoded, err := json.Marshal(fileMetadata)
			if err != nil {
				return err
			}
			if err := bucket.Put([]byte(path), encoded); err != nil {
				return err
			}
		}
		return nil
	})
}

func (store *Store) SavePageToken(token string) error {
	return store.database.Update(func(transaction *bbolt.Tx) error {
		bucket := transaction.Bucket(bucketConfig)
		return bucket.Put(keyPageToken, []byte(token))
	})
}

func (store *Store) ReplaceSyncState(remoteFiles types.DriveFileMap, metadata map[string]*types.FileMetadata, pageToken string) error {
	return store.database.Update(func(transaction *bbolt.Tx) error {
		remoteBucket := transaction.Bucket(bucketRemoteFiles)
		metadataBucket := transaction.Bucket(bucketMetadata)
		if err := clearBucket(remoteBucket); err != nil {
			return err
		}
		if err := clearBucket(metadataBucket); err != nil {
			return err
		}
		for path, driveFile := range remoteFiles {
			encoded, err := json.Marshal(driveFile)
			if err != nil {
				return err
			}
			if err := remoteBucket.Put([]byte(path), encoded); err != nil {
				return err
			}
		}
		for path, fileMetadata := range metadata {
			encoded, err := json.Marshal(fileMetadata)
			if err != nil {
				return err
			}
			if err := metadataBucket.Put([]byte(path), encoded); err != nil {
				return err
			}
		}
		return transaction.Bucket(bucketConfig).Put(keyPageToken, []byte(pageToken))
	})
}

func (store *Store) SaveFileState(path string, driveFile *types.DriveFile, metadata *types.FileMetadata) error {
	return store.database.Update(func(transaction *bbolt.Tx) error {
		encodedFile, err := json.Marshal(driveFile)
		if err != nil {
			return err
		}
		encodedMetadata, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		if err := transaction.Bucket(bucketRemoteFiles).Put([]byte(path), encodedFile); err != nil {
			return err
		}
		return transaction.Bucket(bucketMetadata).Put([]byte(path), encodedMetadata)
	})
}

func clearBucket(bucket *bbolt.Bucket) error {
	cursor := bucket.Cursor()
	for key, _ := cursor.First(); key != nil; key, _ = cursor.Next() {
		if err := cursor.Delete(); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) LoadPageToken() (string, error) {
	var token string
	err := store.database.View(func(transaction *bbolt.Tx) error {
		bucket := transaction.Bucket(bucketConfig)
		value := bucket.Get(keyPageToken)
		if value != nil {
			token = string(value)
		}
		return nil
	})
	return token, err
}
