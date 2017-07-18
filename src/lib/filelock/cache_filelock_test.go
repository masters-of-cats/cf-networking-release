package filelock_test

import (
	"crypto/md5"
	"io"
	"io/ioutil"
	"lib/fakes"
	"lib/filelock"
	"lib/filelock/filelockfakes"
	"os"

	"code.cloudfoundry.org/cli/cf/errors"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("CacheFilelock", func() {
	var (
		fileLockPath   string
		cacheFileLock  *filelock.CacheFileLock
		fakeFileLocker *fakes.FileLocker
	)

	BeforeEach(func() {
		tempFile, err := ioutil.TempFile(os.TempDir(), "fileLock")
		Expect(err).NotTo(HaveOccurred())
		fileLockPath = tempFile.Name()

		fakeFileLocker = &fakes.FileLocker{}
		cacheFileLock = filelock.NewCacheFileLock(fakeFileLocker, fileLockPath, md5.New())
	})

	AfterEach(func() {
		err := os.Remove(fileLockPath)
		Expect(err).NotTo(HaveOccurred())
	})

	Describe("Open", func() {
		var (
			lockedFile      *fakes.LockedFile
			updatedContents []byte
		)

		BeforeEach(func() {
			updatedContents = []byte("dragonfruit")

			lockedFile = &fakes.LockedFile{}
			lockedFile.ReadStub = func(contents []byte) (int, error) {
				for i, v := range updatedContents {
					contents[i] = v
				}
				return len(contents), io.EOF
			}
			fakeFileLocker.OpenReturns(lockedFile, nil)
		})

		It("should use cached data if the container file has not been updated", func() {
			_, err := cacheFileLock.Open()
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeFileLocker.OpenCallCount()).To(Equal(1))

			var cacheLockedFile filelock.LockedFile
			cacheLockedFile, err = cacheFileLock.Open()
			Expect(err).NotTo(HaveOccurred())
			Expect(fakeFileLocker.OpenCallCount()).To(Equal(1))
			Expect(cacheLockedFile).NotTo(BeNil())

			var contents = make([]byte, 11)
			cacheLockedFile.Read(contents)
			Expect(contents).To(Equal(updatedContents))
		})

		Context("when the cached file is out of date", func() {
			BeforeEach(func() {
				lockFile, err := ioutil.TempFile(os.TempDir(), "fake-lock-file")
				Expect(ioutil.WriteFile(lockFile.Name(), []byte("dragonfruit"), os.ModePerm)).To(Succeed())
				Expect(err).NotTo(HaveOccurred())

				lockerWithLockFile := filelock.NewLocker(lockFile.Name())
				cacheFileLock = filelock.NewCacheFileLock(lockerWithLockFile, lockFile.Name(), md5.New())
			})

			It("is able to read from the same cache file multiple times", func() {
				cacheLockedFile, err := cacheFileLock.Open()
				Expect(err).NotTo(HaveOccurred())
				Expect(cacheLockedFile).NotTo(BeNil())

				var contents = make([]byte, 11)
				cacheLockedFile.Read(contents)
				Expect(contents).To(Equal(updatedContents))

				cacheLockedFile, err = cacheFileLock.Open()
				Expect(err).NotTo(HaveOccurred())

				contents = make([]byte, 11)
				cacheLockedFile.Read(contents)
				Expect(contents).To(Equal(updatedContents))
			})

			Context("when the lock file is opened", func() {
				BeforeEach(func() {
					lockFile, err := ioutil.TempFile(os.TempDir(), "updated-lock-file")
					Expect(err).NotTo(HaveOccurred())
					cacheFileLock = filelock.NewCacheFileLock(fakeFileLocker, lockFile.Name(), md5.New())
				})
				It("closes the locked file", func() {
					cacheLockedFile, err := cacheFileLock.Open()
					Expect(err).NotTo(HaveOccurred())
					Expect(cacheLockedFile).NotTo(BeNil())

					Expect(fakeFileLocker.OpenCallCount()).To(Equal(1))
					Expect(lockedFile.CloseCallCount()).To(Equal(1))
				})
			})
		})

		Context("when unable to open a filelocker", func() {
			BeforeEach(func() {
				fakeFileLocker.OpenReturns(nil, errors.New("apple"))
			})

			It("should return an error", func() {
				_, err := cacheFileLock.Open()
				Expect(err).To(MatchError("open file lock: apple"))

				By("checking that the mod time was not updated through attempting to open the locked file again")
				cacheFileLock.Open()
				Expect(fakeFileLocker.OpenCallCount()).To(Equal(2))
			})
		})

		Context("when unable to read the lockedfile", func() {
			BeforeEach(func() {
				fakeLockedFile := &fakes.LockedFile{}
				fakeLockedFile.ReadReturns(0, errors.New("pineapple"))
				fakeFileLocker.OpenReturns(fakeLockedFile, nil)
			})

			It("should return an error", func() {
				_, err := cacheFileLock.Open()
				Expect(err).To(MatchError("read locked file: pineapple"))
			})
		})

		Context("when unable to open the file lock", func() {
			BeforeEach(func() {
				cacheFileLock = filelock.NewCacheFileLock(fakeFileLocker, "some/garbage/path", md5.New())
			})

			It("should return an error", func() {
				_, err := cacheFileLock.Open()
				Expect(err).To(MatchError("open file: open some/garbage/path: no such file or directory"))
			})
		})

		Context("when copying the bytes to the hasher fails", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(fileLockPath, []byte("dragonfruit"), os.ModePerm)).To(Succeed())

				badHasher := &filelockfakes.FakeHash{}
				badHasher.WriteReturns(-1, errors.New("banana"))
				cacheFileLock = filelock.NewCacheFileLock(fakeFileLocker, fileLockPath, badHasher)
			})

			It("should return an error", func() {
				_, err := cacheFileLock.Open()
				Expect(err).To(MatchError("copy bytes: banana"))
			})
		})
	})
})
