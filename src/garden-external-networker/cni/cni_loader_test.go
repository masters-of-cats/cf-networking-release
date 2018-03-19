package cni_test

import (
	"garden-external-networker/cni"
	"io/ioutil"
	"path/filepath"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetNetworkConfigs", func() {
	var (
		cniLoader           *cni.CNILoader
		dir                 string
		err                 error
		expectedNetCfgs     []*types.NetConf
		expectedNetListCfg1 *libcni.NetworkConfigList
		expectedNetListCfg2 *libcni.NetworkConfigList
	)

	BeforeEach(func() {
		dir, err = ioutil.TempDir("", "test-cni-dir")
		Expect(err).NotTo(HaveOccurred())

		cniLoader = &cni.CNILoader{
			PluginDir: "",
			ConfigDir: dir,
		}

		expectedNetCfgs = []*types.NetConf{
			{
				Name: "mynet",
				Type: "bridge",
			},
			{
				Name: "mynet2",
				Type: "vxlan",
			},
		}

		expectedNetListCfg1 = &libcni.NetworkConfigList{
			Name: "mynet",
			Plugins: []*libcni.NetworkConfig{
				{
					Network: &types.NetConf{
						Name: "mynet2",
						Type: "vxlan",
					},
				},
			},
		}

		expectedNetListCfg2 = &libcni.NetworkConfigList{
			Name: "mynet2",
			Plugins: []*libcni.NetworkConfig{
				{
					Network: &types.NetConf{
						Name: "mynet2",
						Type: "vxlan",
					},
				},
			},
		}
	})

	Context("when the config dir does not exist", func() {
		BeforeEach(func() {
			cniLoader.ConfigDir = "/thisdoesnot/exist"
		})
		It("returns a meaningful error", func() {
			_, _, err := cniLoader.GetNetworkConfigs()
			Expect(err).To(MatchError(HavePrefix("error loading config:")))
		})
	})

	Context("when no config files exist in dir", func() {
		It("does not load any netconfig", func() {
			netCfgs, netListCfgs, err := cniLoader.GetNetworkConfigs()
			Expect(err).NotTo(HaveOccurred())
			Expect(netCfgs).To(HaveLen(0))
			Expect(netListCfgs).To(HaveLen(0))
		})
	})

	Context("when a valid config file exists", func() {
		BeforeEach(func() {
			err = ioutil.WriteFile(filepath.Join(dir, "foo.conf"), []byte(`{ "name": "mynet", "type": "bridge" }`), 0600)
			Expect(err).NotTo(HaveOccurred())
		})
		It("loads a single network config", func() {
			netCfgs, netListCfgs, err := cniLoader.GetNetworkConfigs()
			Expect(err).NotTo(HaveOccurred())
			Expect(netCfgs).To(HaveLen(1))
			Expect(netListCfgs).To(HaveLen(0))
			Expect(netCfgs[0].Network).To(Equal(expectedNetCfgs[0]))
		})
	})

	Context("when multple valid config files exists", func() {
		BeforeEach(func() {
			err = ioutil.WriteFile(filepath.Join(dir, "foo.conf"), []byte(`{ "name": "mynet", "type": "bridge" }`), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = ioutil.WriteFile(filepath.Join(dir, "bar.conf"), []byte(`{ "name": "mynet2", "type": "vxlan" }`), 0600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("loads all network configs", func() {
			netCfgs, netListCfgs, err := cniLoader.GetNetworkConfigs()
			Expect(err).NotTo(HaveOccurred())
			Expect(netCfgs).To(HaveLen(2))
			Expect(netListCfgs).To(HaveLen(0))
			Expect(netCfgs[0].Network).To(Equal(expectedNetCfgs[1]))
			Expect(netCfgs[1].Network).To(Equal(expectedNetCfgs[0]))
		})
	})

	Context("when multple valid config list files exists", func() {
		BeforeEach(func() {
			err = ioutil.WriteFile(filepath.Join(dir, "foo.conflist"), []byte(`{ "name": "mynet", "plugins": [{ "name": "mynet2", "type": "vxlan" }] }`), 0600)
			Expect(err).NotTo(HaveOccurred())
			err = ioutil.WriteFile(filepath.Join(dir, "bar.conflist"), []byte(`{ "name": "mynet2", "plugins": [{ "name": "mynet2", "type": "vxlan" }] }`), 0600)
			Expect(err).NotTo(HaveOccurred())
		})

		It("loads all network configs", func() {
			_, netListCfgs, err := cniLoader.GetNetworkConfigs()
			Expect(err).NotTo(HaveOccurred())
			Expect(netListCfgs).To(HaveLen(2))
			Expect(netListCfgs[0].Name).To(Equal(expectedNetListCfg2.Name))
			Expect(netListCfgs[0].Plugins[0].Network).To(Equal(expectedNetListCfg2.Plugins[0].Network))
			Expect(netListCfgs[1].Name).To(Equal(expectedNetListCfg1.Name))
			Expect(netListCfgs[1].Plugins[0].Network).To(Equal(expectedNetListCfg1.Plugins[0].Network))
		})
	})
})
