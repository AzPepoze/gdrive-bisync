package appstate

import (
	"fmt"
	"os"
)

type InstanceLock struct {
	file *os.File
}

func AcquireInstanceLock(path string) (*InstanceLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	locked, err := tryLockFile(file)
	if err != nil {
		file.Close()
		return nil, err
	}
	if !locked {
		file.Close()
		return nil, fmt.Errorf("another gdrive-bisync process already owns %s", path)
	}

	if err := file.Truncate(0); err != nil {
		file.Close()
		return nil, err
	}
	if _, err := fmt.Fprintf(file, "%d\n", os.Getpid()); err != nil {
		file.Close()
		return nil, err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, err
	}
	return &InstanceLock{file: file}, nil
}

func (lock *InstanceLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unlockFile(lock.file)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
