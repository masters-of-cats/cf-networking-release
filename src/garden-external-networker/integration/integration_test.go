package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"code.cloudfoundry.org/netplugin-shim/message"
	"github.com/containernetworking/plugins/pkg/ns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"golang.org/x/sys/unix"
)

type fakePluginLogData struct {
	Args  []string
	Env   map[string]string
	Stdin string
}

func expectedStdin_CNI_ADD(index int) string {
	return fmt.Sprintf(`
	{
		"cniVersion": "0.1.0",
		"name": "some-net-%d",
		"type": "plugin-%d",
		"runtimeConfig": {
			"portMappings": [
				{"host_port": 12345, "container_port": 7000},
				{"host_port": 60000, "container_port": 7000}
			],
			"netOutRules": [{
				"protocol": 1,
				"networks": [
					{"start": "8.8.8.8", "end": "9.9.9.9"}
				],
				"ports": [
					{"start": 53, "end": 54}
				],
				"log": true
			}]
		},
		"metadata": {
				"some-key": "some-value",
				"policy_group_id": "some-group-id"
		}
	}`, index, index)
}
func expectedStdin_CNI_DEL(index int) string {
	return fmt.Sprintf(`
	{
		"cniVersion": "0.1.0",
		"name": "some-net-%d",
		"type": "plugin-%d"
	}`, index, index)
}

func writeConfig(index int, outDir string) error {
	config := fmt.Sprintf(`
	{
		"cniVersion": "0.1.0",
		"name": "some-net-%d",
		"type": "plugin-%d"
	}`, index, index)
	outpath := filepath.Join(outDir, fmt.Sprintf("%d-plugin-%d.conf", 10*index, index))
	return ioutil.WriteFile(outpath, []byte(config), 0600)
}

func sameFile(path1, path2 string) bool {
	file1, err := os.Stat(path1)
	Expect(err).NotTo(HaveOccurred())

	file2, err := os.Stat(path2)
	Expect(err).NotTo(HaveOccurred())
	return os.SameFile(file1, file2)
}

const DEFAULT_TIMEOUT = "10s"
const GlobalIPTablesLockFile = "/tmp/netman/iptables.lock"

func buildStdin(inputs interface{}) io.Reader {
	jsonBytes, err := json.Marshal(inputs)
	Expect(err).NotTo(HaveOccurred())
	return bytes.NewReader(jsonBytes)
}

func containerIPTablesRules(containerNetns string, tableName string) []string {
	iptablesSession, err := gexec.Start(exec.Command("ip", "netns", "exec", containerNetns, "iptables", "-w", "-S", "-t", tableName), GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(iptablesSession).Should(gexec.Exit(0))
	return strings.Split(string(iptablesSession.Out.Contents()), "\n")
}

var _ = Describe("Garden External Networker", func() {
	var (
		config                 map[string]interface{}
		cniConfigDir           string
		fakePid                int
		fakeLogDir             string
		expectedNetNSPath      string
		bindMountRoot          string
		stateFilePath          string
		containerHandle        string
		containerNetNS         ns.NetNS
		containerNSShortName   string
		proxyRedirectCIDR      string
		fakeProcess            *os.Process
		fakeConfigFilePath     string
		upCommand, downCommand *exec.Cmd
	)

	BeforeEach(func() {
		var err error
		cniConfigDir, err = ioutil.TempDir("", "cni-config-")
		Expect(err).NotTo(HaveOccurred())

		fakeLogDir, err = ioutil.TempDir("", "fake-logs-")
		Expect(err).NotTo(HaveOccurred())

		containerHandle = fmt.Sprintf("container-%04x-%x", GinkgoParallelNode(), rand.Int63())

		sleepCmd := exec.Command("sleep", "1000")
		if runtime.GOOS == "windows" {
			sleepCmd = exec.Command("powershell", "Start-Sleep", "1000")
		}

		containerNetNS = createNetworkNamespace()
		containerNSShortName = filepath.Base(containerNetNS.Path())

		Expect(containerNetNS.Do(func(_ ns.NetNS) error {
			err := sleepCmd.Start()
			fakeProcess = sleepCmd.Process
			return err
		})).To(Succeed())

		fakePid = fakeProcess.Pid

		bindMountRoot, err = ioutil.TempDir("", "bind-mount-root")
		Expect(err).NotTo(HaveOccurred())

		expectedNetNSPath = filepath.Join(bindMountRoot, containerHandle)

		stateFile, err := ioutil.TempFile("", "external-networker-state.json")
		Expect(err).NotTo(HaveOccurred())
		Expect(stateFile.Close()).To(Succeed())
		stateFilePath = stateFile.Name()

		Expect(writeConfig(0, cniConfigDir)).To(Succeed())
		Expect(writeConfig(1, cniConfigDir)).To(Succeed())
		Expect(writeConfig(2, cniConfigDir)).To(Succeed())

		configFile, err := ioutil.TempFile("", "adapter-config-")
		Expect(err).NotTo(HaveOccurred())
		fakeConfigFilePath = configFile.Name()
		proxyRedirectCIDR = "10.255.0.0/16"

		config = map[string]interface{}{
			"cni_plugin_dir":      paths.CniPluginDir,
			"cni_config_dir":      cniConfigDir,
			"bind_mount_dir":      bindMountRoot,
			"iptables_lock_file":  GlobalIPTablesLockFile,
			"proxy_redirect_cidr": "",
			"proxy_port":          9999,
			"proxy_uid":           42,
			"state_file":          stateFilePath,
			"start_port":          60000,
			"total_ports":         56,
			"log_prefix":          "cfnetworking",
			"search_domains": []string{
				"pivotal.io",
				"foo.bar",
				"baz.me",
			},
		}
		configBytes, err := json.Marshal(config)
		Expect(err).NotTo(HaveOccurred())
		_, err = configFile.Write(configBytes)
		Expect(err).NotTo(HaveOccurred())
		Expect(configFile.Close()).To(Succeed())

		upCommand = exec.Command(paths.PathToAdapter)
		upCommand.Env = append(os.Environ(), "FAKE_LOG_DIR="+fakeLogDir)
		upCommand.Stdin = buildStdin(map[string]interface{}{
			"pid": fakePid,
			"properties": map[string]string{
				"some-key":        "some-value",
				"policy_group_id": "some-group-id",
			},
			"netin": []map[string]int{
				{
					"host_port":      12345,
					"container_port": 7000,
				},
				{
					"host_port":      0,
					"container_port": 7000,
				},
			},
			"netout_rules": []map[string]interface{}{
				{
					"protocol": 1,
					"networks": []map[string]string{
						{
							"start": "8.8.8.8",
							"end":   "9.9.9.9",
						},
					},
					"ports": []map[string]int{
						{
							"start": 53,
							"end":   54,
						},
					},
					"log": true,
				},
			},
		},
		)
		upCommand.Args = []string{
			paths.PathToAdapter,
			"--configFile", fakeConfigFilePath,
			"--action", "up",
			"--handle", containerHandle,
		}

		downCommand = exec.Command(paths.PathToAdapter)
		downCommand.Env = append(os.Environ(), "FAKE_LOG_DIR="+fakeLogDir)
		downCommand.Stdin = strings.NewReader(`{}`)
		downCommand.Args = []string{
			paths.PathToAdapter,
			"--action", "down",
			"--handle", containerHandle,
			"--configFile", fakeConfigFilePath,
		}
	})

	AfterEach(func() {
		removeNetworkNamespace(containerNetNS)

		Expect(os.Remove(fakeConfigFilePath)).To(Succeed())
		Expect(os.RemoveAll(cniConfigDir)).To(Succeed())
		Expect(os.RemoveAll(fakeLogDir)).To(Succeed())
		Expect(fakeProcess.Kill()).To(Succeed())
	})

	Context("when proxy_redirect_cidr is empty", func() {
		It("does not write iptables rules in the container", func() {
			runAndWait(upCommand)

			Expect(containerIPTablesRules(containerNSShortName, "nat")).NotTo(ContainElement(ContainSubstring("REDIRECT")))
		})
	})

	Context("when the proxy_redirect_cidr is set", func() {
		BeforeEach(func() {
			config["proxy_redirect_cidr"] = proxyRedirectCIDR
			configBytes, err := json.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = ioutil.WriteFile(fakeConfigFilePath, configBytes, 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should setup proxy iptable rules inside the container network namespace", func() {
			runAndWait(upCommand)

			Expect(containerIPTablesRules(containerNSShortName, "nat")).To(ContainElement("-A OUTPUT -d 10.255.0.0/16 -p tcp -m owner ! --uid-owner 42 -j REDIRECT --to-ports 9999"))
		})
	})

	It("should call CNI ADD and DEL", func() {
		By("calling up")
		upSession := runAndWait(upCommand)
		Expect(upSession.Out.Contents()).To(MatchJSON(`{
			"properties": {
				"garden.network.container-ip": "169.254.1.2",
				"garden.network.host-ip": "255.255.255.255",
				"garden.network.mapped-ports": "[{\"HostPort\":12345,\"ContainerPort\":7000},{\"HostPort\":60000,\"ContainerPort\":7000}]"
			},
			"dns_servers": [
				"1.2.3.4"
			],
			"search_domains": [
				"pivotal.io",
				"foo.bar",
				"baz.me"
			]
		}`))

		By("checking that the first CNI plugin in the plugin directory got called with ADD")
		logFileContents, err := ioutil.ReadFile(filepath.Join(fakeLogDir, "plugin-0.log"))
		Expect(err).NotTo(HaveOccurred())
		var pluginCallInfo fakePluginLogData
		Expect(json.Unmarshal(logFileContents, &pluginCallInfo)).To(Succeed())

		Expect(pluginCallInfo.Stdin).To(MatchJSON(expectedStdin_CNI_ADD(0)))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_COMMAND", "ADD"))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_CONTAINERID", containerHandle))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_IFNAME", "eth0"))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_PATH", paths.CniPluginDir))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_NETNS", expectedNetNSPath))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_ARGS", ""))

		if runtime.GOOS != "windows" {
			By("checking that the fake process's network namespace has been bind-mounted into the filesystem")
			Expect(sameFile(expectedNetNSPath, fmt.Sprintf("/proc/%d/ns/net", fakePid))).To(BeTrue())
		}

		By("calling down")
		runAndWait(downCommand)

		By("checking that the first CNI plugin in the plugin directory got called with DEL")
		logFileContents, err = ioutil.ReadFile(filepath.Join(fakeLogDir, "plugin-0.log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(json.Unmarshal(logFileContents, &pluginCallInfo)).To(Succeed())

		Expect(pluginCallInfo.Stdin).To(MatchJSON(expectedStdin_CNI_DEL(0)))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_COMMAND", "DEL"))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_CONTAINERID", containerHandle))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_IFNAME", "eth0"))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_PATH", paths.CniPluginDir))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_NETNS", expectedNetNSPath))
		Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_ARGS", ""))

		if runtime.GOOS != "windows" {
			By("checking that the bind-mounted namespace has been removed")
			Expect(expectedNetNSPath).NotTo(BeAnExistingFile())
		}

		By("seeing that is succeeds when calling down again")
		downCommand2 := exec.Command(paths.PathToAdapter)
		downCommand2.Env = append(os.Environ(), "FAKE_LOG_DIR="+fakeLogDir)
		downCommand2.Stdin = strings.NewReader(`{}`)
		downCommand2.Args = []string{
			paths.PathToAdapter,
			"--action", "down",
			"--handle", containerHandle,
			"--configFile", fakeConfigFilePath,
		}
		runAndWait(downCommand2)
	})

	Context("when the CNI plugin result DNS servers list is empty", func() {
		BeforeEach(func() {
			upCommand.Env = append(upCommand.Env, "FAKE_CNI_DEBUG=no_dns_result")
		})

		It("omits the 'dns_servers' field from the Network ('up') output", func() {
			// this behavior is necessary in order for Garden to fall back to using
			// the host's /etc/resolv.conf.
			upSession := runAndWait(upCommand)
			Expect(upSession.Out.Contents()).To(MatchJSON(`{
			"properties": {
				"garden.network.container-ip": "169.254.1.2",
				"garden.network.host-ip": "255.255.255.255",
				"garden.network.mapped-ports": "[{\"HostPort\":12345,\"ContainerPort\":7000},{\"HostPort\":60000,\"ContainerPort\":7000}]"
			},
			"search_domains": [
				"pivotal.io",
				"foo.bar",
				"baz.me"
			]
		}`))

		})
	})

	Context("when the configuration search_domains list is empty", func() {
		BeforeEach(func() {
			delete(config, "search_domains")
			configBytes, err := json.Marshal(config)
			Expect(err).NotTo(HaveOccurred())
			err = ioutil.WriteFile(fakeConfigFilePath, configBytes, 0644)
			Expect(err).NotTo(HaveOccurred())
		})

		It("omits the 'search_domains' field from the Network ('up') output", func() {
			// this behavior is necessary in order for Garden to fall back to using
			// the host's /etc/resolv.conf.
			upSession := runAndWait(upCommand)
			Expect(upSession.Out.Contents()).To(MatchJSON(`{
			"properties": {
				"garden.network.container-ip": "169.254.1.2",
				"garden.network.host-ip": "255.255.255.255",
				"garden.network.mapped-ports": "[{\"HostPort\":12345,\"ContainerPort\":7000},{\"HostPort\":60000,\"ContainerPort\":7000}]"
			},
			"dns_servers": [
				"1.2.3.4"
			]
		}`))

		})
	})

	Context("when run as a daemon", func() {
		var (
			tmpDir     string
			socketPath string
			session    *gexec.Session
		)

		BeforeEach(func() {
			var err error

			tmpDir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())
			socketPath = filepath.Join(tmpDir, "test-garden-external-networker.sock")

			upCommand.Args = append(upCommand.Args, "--socket", socketPath)
			upCommand.Stdin = nil

			session, err = gexec.Start(upCommand, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(socketPath).Should(BeAnExistingFile())
		})

		AfterEach(func() {
			session.Terminate()
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		It("deletes the socket on exit", func() {
			session.Interrupt()
			Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit())
			Expect(socketPath).NotTo(BeAnExistingFile())
		})

		It("should call CNI ADD and DEL", func() {
			By("calling up")
			connection, err := net.Dial("unix", socketPath)
			Expect(err).NotTo(HaveOccurred())

			fakeShim := FakeShim{
				connection:     connection,
				containerNetNS: containerNetNS,
			}

			data, err := json.Marshal(map[string]interface{}{
				"pid": fakePid,
				"netin": []map[string]int{
					{
						"host_port":      12345,
						"container_port": 7000,
					},
					{
						"host_port":      60000,
						"container_port": 7000,
					},
				},
				"netout_rules": []map[string]interface{}{
					{
						"protocol": 1,
						"networks": []map[string]string{
							{
								"start": "8.8.8.8",
								"end":   "9.9.9.9",
							},
						},
						"ports": []map[string]int{
							{
								"start": 53,
								"end":   54,
							},
						},
						"log": true,
					},
				},
				"properties": map[string]interface{}{
					"some-key":        "some-value",
					"policy_group_id": "some-group-id",
				},
			})
			Expect(err).NotTo(HaveOccurred())

			upMessage := message.Message{
				Command: []byte("up"),
				Handle:  []byte(containerHandle),
				Data:    data,
			}
			fakeShim.sendSocketMessage(upMessage)

			decoder := json.NewDecoder(connection)
			response := make(map[string]interface{})
			Expect(decoder.Decode(&response)).To(Succeed())
			data, err = json.Marshal(response)
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(MatchJSON(`{
				"dns_servers": [
					"1.2.3.4"
				],
				"properties": {
					"garden.network.container-ip": "169.254.1.2",
					"garden.network.host-ip": "255.255.255.255",
					"garden.network.mapped-ports": "[{\"HostPort\":12345,\"ContainerPort\":7000},{\"HostPort\":60000,\"ContainerPort\":7000}]"
				},
				"search_domains": [
					"pivotal.io",
					"foo.bar",
					"baz.me"
				]
			}`))

			Expect(connection.Close()).To(Succeed())

			By("checking that the first CNI plugin in the plugin directory got called with ADD")
			Eventually(filepath.Join(fakeLogDir, "plugin-0.log")).Should(BeAnExistingFile())
			logFileContents := readFile(filepath.Join(fakeLogDir, "plugin-0.log"))
			Expect(os.Remove(filepath.Join(fakeLogDir, "plugin-0.log"))).To(Succeed())
			var pluginCallInfo fakePluginLogData
			Expect(json.Unmarshal(logFileContents, &pluginCallInfo)).To(Succeed())

			Expect(pluginCallInfo.Stdin).To(MatchJSON(expectedStdin_CNI_ADD(0)))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_COMMAND", "ADD"))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_CONTAINERID", containerHandle))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_IFNAME", "eth0"))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_PATH", paths.CniPluginDir))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_NETNS", expectedNetNSPath))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_ARGS", ""))

			if runtime.GOOS != "windows" {
				By("checking that the fake process's network namespace has been bind-mounted into the filesystem")
				Expect(sameFile(expectedNetNSPath, fmt.Sprintf("/proc/%d/ns/net", fakePid))).To(BeTrue())
			}

			By("calling down")
			connection2, err := net.Dial("unix", socketPath)
			Expect(err).NotTo(HaveOccurred())

			fakeShim2 := FakeShim{
				connection:     connection2,
				containerNetNS: containerNetNS,
			}
			downMessage := message.Message{
				Command: []byte("down"),
				Handle:  []byte(containerHandle),
				Data:    []byte("{}"),
			}
			fakeShim2.sendSocketMessage(downMessage)

			decoder = json.NewDecoder(connection2)
			response = make(map[string]interface{})
			Expect(decoder.Decode(&response)).To(Succeed())

			By("checking that the first CNI plugin in the plugin directory got called with DEL")
			Eventually(filepath.Join(fakeLogDir, "plugin-0.log")).Should(BeAnExistingFile())
			logFileContents = readFile(filepath.Join(fakeLogDir, "plugin-0.log"))
			Expect(json.Unmarshal(logFileContents, &pluginCallInfo)).To(Succeed())

			stdinMap := make(map[string]interface{})
			Expect(json.Unmarshal([]byte(pluginCallInfo.Stdin), &stdinMap)).To(Succeed())

			Expect(stdinMap).To(HaveKeyWithValue("cniVersion", "0.1.0"))
			Expect(stdinMap).To(HaveKeyWithValue("type", "plugin-0"))
			Expect(stdinMap).To(HaveKeyWithValue("name", "some-net-0"))

			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_COMMAND", "DEL"))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_CONTAINERID", containerHandle))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_IFNAME", "eth0"))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_PATH", paths.CniPluginDir))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_NETNS", expectedNetNSPath))
			Expect(pluginCallInfo.Env).To(HaveKeyWithValue("CNI_ARGS", ""))

			if runtime.GOOS != "windows" {
				By("checking that the bind-mounted namespace has been removed")
				Expect(expectedNetNSPath).NotTo(BeAnExistingFile())
			}

			Expect(connection2.Close()).To(Succeed())

			By("seeing that is succeeds when calling down again")
			connection3, err := net.Dial("unix", socketPath)
			Expect(err).NotTo(HaveOccurred())

			fakeShim3 := FakeShim{
				connection:     connection3,
				containerNetNS: containerNetNS,
			}
			downMessage = message.Message{
				Command: []byte("down"),
				Handle:  []byte(containerHandle),
				Data:    []byte("{}"),
			}
			fakeShim3.sendSocketMessage(downMessage)

			decoder = json.NewDecoder(connection3)
			response = make(map[string]interface{})
			Expect(decoder.Decode(&response)).To(Succeed())

			Expect(connection3.Close()).To(Succeed())
		})
	})
})

func runAndWait(cmd *exec.Cmd) *gexec.Session {
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, DEFAULT_TIMEOUT).Should(gexec.Exit(0))
	return session
}

func createNetworkNamespace() ns.NetNS {
	networkNS, err := ns.NewNS()
	Expect(err).ToNot(HaveOccurred())
	return networkNS
}

func removeNetworkNamespace(containerNetNs ns.NetNS) {
	Expect(containerNetNs.Close()).To(Succeed())
}

type FakeShim struct {
	containerNetNS ns.NetNS
	connection     net.Conn
}

func (f *FakeShim) sendSocketMessage(msg message.Message) {
	socketControlMessage := unix.UnixRights(int(f.containerNetNS.Fd()))
	socket := f.connection.(*net.UnixConn)
	_, _, err := socket.WriteMsgUnix(nil, socketControlMessage, nil)
	Expect(err).NotTo(HaveOccurred())

	encoder := json.NewEncoder(f.connection)
	Expect(encoder.Encode(msg)).To(Succeed())
}
