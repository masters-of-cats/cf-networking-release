package ipc_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	. "garden-external-networker/ipc"
	"garden-external-networker/ipc/ipcfakes"
	"garden-external-networker/manager"
)

var _ = Describe("IpcMux", func() {
	var (
		mux           *Mux
		smFake        *ipcfakes.FakeSocketManager
		socketTmpFile string
		shimKCmd      *exec.Cmd
		shimKSession  *gexec.Session
	)

	BeforeEach(func() {
		up := func(handle string, inputs manager.UpInputs) (*manager.UpOutputs, error) {
			return nil, nil
		}
		down := func(handle string) error {
			return nil
		}
		var err error
		tmpDir, err := ioutil.TempDir("", "")
		Expect(err).NotTo(HaveOccurred())
		socketTmpFile = filepath.Join(tmpDir, "test-garden-external-networker.sock")
		smFake = new(ipcfakes.FakeSocketManager)
		mux = NewMux(up, down)
		mux.SocketManager = smFake

		shimKCmd = exec.Command(shimKPath, "--socket", socketTmpFile)
		shimKSession, err = gexec.Start(shimKCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		Expect(os.Remove(socketTmpFile)).To(Succeed())
	})

	It("calls ReadFileDescriptor", func() {
		go func() {
			mux.HandleWithSocket(os.Stdout, socketTmpFile)
		}()
		defer GinkgoRecover()
		Eventually(socketTmpFile).Should(BeAnExistingFile())
		go func() {
			Expect(shimKSession.Wait()).To(gexec.Exit())
		}()
		defer GinkgoRecover()

		mux.KillChannel <- syscall.SIGINT

		Expect(smFake.ReadFileDescriptorCallCount()).To(Equal(1))
	})

	It("calls ReadMessage", func() {
		go func() {
			mux.HandleWithSocket(os.Stdout, socketTmpFile)
		}()
		defer GinkgoRecover()
		Eventually(socketTmpFile).Should(BeAnExistingFile())
		go func() {
			Expect(shimKSession.Wait()).To(gexec.Exit())
		}()
		defer GinkgoRecover()

		mux.KillChannel <- syscall.SIGINT

		Expect(smFake.ReadMessageCallCount()).To(Equal(1))
	})

})
