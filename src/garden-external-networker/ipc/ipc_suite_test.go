package ipc_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestIpc(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ipc Suite")
}

var shimKPath string

var _ = BeforeSuite(func() {
	var err error
	shimKPath, err = gexec.Build("garden-external-networker/ipc/shimkardashian")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
