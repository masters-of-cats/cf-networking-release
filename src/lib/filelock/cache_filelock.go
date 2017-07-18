package filelock

import (
	"bytes"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"os"
	"time"
)

//go:generate counterfeiter -o ./fakes/hash.go --fake-name FakeHash hash.Hash

type CacheFileLock struct {
	fileLocker      FileLocker
	fileLockPath    string
	fileLockModTime time.Time
	hash            string
	hasher          hash.Hash
	cacheFile       []byte
}

func NewCacheFileLock(fileLocker FileLocker, fileLockPath string, hasher hash.Hash) *CacheFileLock {
	return &CacheFileLock{
		fileLocker:   fileLocker,
		fileLockPath: fileLockPath,
		hasher:       hasher,
	}
}

type InMemoryLockedFile struct {
	*bytes.Reader
}

func (InMemoryLockedFile) Close() error {
	return nil
}

func (InMemoryLockedFile) Truncate(int64) error {
	return nil
}

func (InMemoryLockedFile) Write([]byte) (int, error) {
	panic("Not Implemented")
}

func (c *CacheFileLock) Open() (LockedFile, error) {
	currentHash, err := c.currentHash()
	if err != nil {
		return nil, err
	}

	if c.hash != currentHash {
		lockedFile, err := c.fileLocker.Open()
		if err != nil {
			return nil, fmt.Errorf("open file lock: %s", err)
		}
		defer lockedFile.Close()
		lockedFileContents, err := ioutil.ReadAll(lockedFile)
		if err != nil {
			return nil, fmt.Errorf("read locked file: %s", err)
		}
		c.hash = currentHash
		c.cacheFile = lockedFileContents
	}

	return InMemoryLockedFile{bytes.NewReader(c.cacheFile)}, nil
}

func (c *CacheFileLock) currentHash() (string, error) {
	f, err := os.Open(c.fileLockPath)
	if err != nil {
		return "", fmt.Errorf("open file: %s", err)
	}
	defer f.Close()

	_, err = io.Copy(c.hasher, f)
	if err != nil {
		return "", fmt.Errorf("copy bytes: %s", err)
	}

	return string(c.hasher.Sum(nil)), nil
}
