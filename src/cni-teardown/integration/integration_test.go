package integration_test

import (
	"cni-wrapper-plugin/lib"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"code.cloudfoundry.org/silk/lib/adapter"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	DEFAULT_TIMEOUT = "5s"

	config                *lib.WrapperConfig
	netlinkAdapter        *adapter.NetlinkAdapter
	ifbName               string
	notSilkCreatedIFBName string
	dummyName             string
	configFilePath        string
	datastorePath         string
	delegateDataDirPath   string
	delegateDatastorePath string
)

var _ = BeforeEach(func() {
	var err error
	ifbName = fmt.Sprintf("i-some-ifb-%d", GinkgoParallelNode())
	dummyName = fmt.Sprintf("ilololol-%d", GinkgoParallelNode())
	notSilkCreatedIFBName = fmt.Sprintf("other-ifb-%d", GinkgoParallelNode())

	netlinkAdapter = &adapter.NetlinkAdapter{}

	// /var/vcap/data/container-metadata
	datastorePath, err = ioutil.TempDir(os.TempDir(), fmt.Sprintf("container-metadata-%d", GinkgoParallelNode()))
	Expect(err).NotTo(HaveOccurred())

	// /var/vcap/data/host-local
	delegateDataDirPath, err = ioutil.TempDir(os.TempDir(), fmt.Sprintf("host-local-%d", GinkgoParallelNode()))
	Expect(err).NotTo(HaveOccurred())

	// /var/vcap/data/silk/store.json
	delegateDatastorePath, err = ioutil.TempDir(os.TempDir(), fmt.Sprintf("silk-%d", GinkgoParallelNode()))
	Expect(err).NotTo(HaveOccurred())

	config = &lib.WrapperConfig{
		Datastore: filepath.Join(datastorePath, "store.json"),
		Delegate: map[string]interface{}{
			"dataDir":   delegateDataDirPath,
			"datastore": filepath.Join(delegateDatastorePath, "store.json"),
		},
		IPTablesLockFile:              "does_not_matter",
		InstanceAddress:               "does_not_matter",
		IngressTag:                    "does_not_matter",
		VTEPName:                      "does_not_matter",
		IPTablesDeniedLogsPerSec:      2,
		IPTablesAcceptedUDPLogsPerSec: 2,
	}
	// write config, pass it as flag to when we call teardown
	configFilePath = writeConfigFile(*config)

	mustSucceed("ip", "link", "add", ifbName, "type", "ifb")
	mustSucceed("ip", "link", "add", notSilkCreatedIFBName, "type", "ifb")
	mustSucceed("ip", "link", "add", dummyName, "type", "dummy")
})

var _ = AfterEach(func() {
	exec.Command("ip", "link", "del", ifbName).Run()
	mustSucceed("ip", "link", "del", notSilkCreatedIFBName)
	mustSucceed("ip", "link", "del", dummyName)

	Expect(os.Remove(configFilePath)).To(Succeed())
})

var _ = FDescribe("Teardown", func() {
	It("destroys only leftover IFB devices and removes the unneeded directories", func() {
		By("running teardown")
		session := runTeardown()
		Expect(session).To(gexec.Exit(0))
		Expect(session.Out.Contents()).To(ContainSubstring("cni-teardown.starting"))

		By("verifying that the ifb is no longer present")
		_, err := netlinkAdapter.LinkByName(ifbName)
		Expect(err).To(MatchError("Link not found"))

		By("verifying that the other devices are not cleaned up")
		_, err = netlinkAdapter.LinkByName(dummyName)
		Expect(err).NotTo(HaveOccurred())

		_, err = netlinkAdapter.LinkByName(notSilkCreatedIFBName)
		Expect(err).NotTo(HaveOccurred())

		By("verifying that the relevant directories no longer exist")
		Expect(fileExists(datastorePath)).To(BeFalse())
		Expect(fileExists(delegateDataDirPath)).To(BeFalse())
		Expect(fileExists(delegateDatastorePath)).To(BeFalse())

		Expect(session.Out.Contents()).To(ContainSubstring("cni-teardown.complete"))
	})

	Context("when the config file does not exist", func() {
		It("logs the errors but still cleans up devices", func() {
		})
	})
	Context("when the config file exists but cannot be read", func() {
		It("logs the errors but still cleans up devices", func() {
		})
	})

	PContext("when the directories to clean up do not exist", func() {
		BeforeEach(func() {
			_, err := os.OpenFile(filepath.Join(delegateDatastorePath, "store.json"), os.O_RDWR|os.O_CREATE, 0400)
			Expect(err).NotTo(HaveOccurred())
		})

		It("logs the errors but still cleans up devices", func() {
			By("running teardown")
			session := runTeardown()
			Expect(session).To(gexec.Exit(0))

			By("verifying that the ifb is no longer present")
			_, err := netlinkAdapter.LinkByName(ifbName)
			Expect(err).To(MatchError("Link not found"))

			By("checking the logs")
			Expect(session.Out.Contents()).To(ContainSubstring("cni-teardown.starting"))
			Expect(session.Out.Contents()).To(ContainSubstring("cni-teardown.failed-to-remove-datastore-path"))
			Expect(session.Out.Contents()).To(ContainSubstring("cni-teardown.failed-to-remove-delegate-datastore-path"))
			Expect(session.Out.Contents()).To(ContainSubstring("cni-teardown.failed-to-remove-delegate-data-dir-path"))
			Expect(session.Out.Contents()).To(ContainSubstring("cni-teardown.complete"))
		})
	})
})

func fileExists(path string) bool {
	_, err := os.Stat(path)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}
func writeConfigFile(config lib.WrapperConfig) string {
	configFile, err := ioutil.TempFile("", "test-config")
	Expect(err).NotTo(HaveOccurred())

	configBytes, err := json.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	err = ioutil.WriteFile(configFile.Name(), configBytes, os.ModePerm)
	Expect(err).NotTo(HaveOccurred())

	return configFile.Name()
}

func mustSucceed(binary string, args ...string) string {
	cmd := exec.Command(binary, args...)
	sess, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(sess, "10s").Should(gexec.Exit(0))
	return string(sess.Out.Contents())
}

func runTeardown() *gexec.Session {
	startCmd := exec.Command(paths.TeardownBin, "--config", configFilePath)
	session, err := gexec.Start(startCmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
	return session
}
