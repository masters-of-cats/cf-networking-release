package proxy_test

import (
	"errors"
	"garden-external-networker/fakes"
	"garden-external-networker/proxy"
	lib_fakes "lib/fakes"
	"lib/rules"
	"strconv"

	"github.com/containernetworking/plugins/pkg/ns"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

//go:generate counterfeiter -o ../fakes/netNS.go --fake-name NetNS . netNS
type netNS interface {
	ns.NetNS
}

var _ = Describe("Redirect", func() {
	var (
		proxyRedirect    *proxy.Redirect
		iptablesAdapter  *lib_fakes.IPTablesAdapter
		namespaceAdapter *fakes.NamespaceAdapter
		netNS            *fakes.NetNS

		containerNetNamespace string
		chainName             string
		redirectCIDR          string
		proxyPort             int
		proxyUID              int
	)

	BeforeEach(func() {
		iptablesAdapter = &lib_fakes.IPTablesAdapter{}
		namespaceAdapter = &fakes.NamespaceAdapter{}
		netNS = &fakes.NetNS{}
		netNS.DoStub = func(toRun func(ns.NetNS) error) error {
			return toRun(netNS)
		}

		namespaceAdapter.GetNSReturns(netNS, nil)

		containerNetNamespace = "some-network-namespace"
		chainName = "some-proxy-chain-name"
		redirectCIDR = "10.255.0.0/24"
		proxyPort = 1111
		proxyUID = 1

		proxyRedirect = &proxy.Redirect{
			IPTables:         iptablesAdapter,
			NamespaceAdapter: namespaceAdapter,
			ChainName:        chainName,
			RedirectCIDR:     redirectCIDR,
			ProxyPort:        proxyPort,
			ProxyUID:         proxyUID,
		}
	})

	Describe("Apply", func() {
		It("apply iptables rules to redirect traffic to the proxy in the container net namespace", func() {
			err := proxyRedirect.Apply(containerNetNamespace)
			Expect(err).NotTo(HaveOccurred())

			Expect(namespaceAdapter.GetNSCallCount()).To(Equal(1))
			Expect(namespaceAdapter.GetNSArgsForCall(0)).To(Equal(containerNetNamespace))

			Expect(netNS.DoCallCount()).To(Equal(1))

			Expect(iptablesAdapter.NewChainCallCount()).To(Equal(1))
			table, name := iptablesAdapter.NewChainArgsForCall(0)
			Expect(table).To(Equal("nat"))
			Expect(name).To(Equal(chainName))

			Expect(iptablesAdapter.BulkAppendCallCount()).To(Equal(2))
			table, name, iptablesRules := iptablesAdapter.BulkAppendArgsForCall(0)
			Expect(table).To(Equal("nat"))
			Expect(name).To(Equal("OUTPUT"))
			Expect(iptablesRules).To(Equal([]rules.IPTablesRule{
				{"-j", chainName},
			}))

			table, name, iptablesRules = iptablesAdapter.BulkAppendArgsForCall(1)
			Expect(table).To(Equal("nat"))
			Expect(name).To(Equal(chainName))
			Expect(iptablesRules).To(Equal([]rules.IPTablesRule{
				{"-m", "owner", "--uid-owner", string(strconv.Itoa(proxyUID)), "-j", "RETURN"},
				{"-d", redirectCIDR, "-p", "tcp", "-j", "REDIRECT", "--to-ports", string(strconv.Itoa(proxyPort))},
			}))
		})

		Context("when creating a new chain fails", func() {
			BeforeEach(func() {
				iptablesAdapter.NewChainReturns(errors.New("banana"))
			})

			It("returns an error", func() {
				err := proxyRedirect.Apply(containerNetNamespace)
				Expect(err).To(MatchError("do in container: creating chain: banana"))
			})
		})

		Context("when bulk appending to OUTPUT fails", func() {
			BeforeEach(func() {
				iptablesAdapter.BulkAppendReturns(errors.New("banana"))
			})

			It("returns an error", func() {
				err := proxyRedirect.Apply(containerNetNamespace)
				Expect(err).To(MatchError("do in container: appending to OUTPUT: banana"))
			})
		})

		Context("when bulk appending to the proxy chain fails", func() {
			BeforeEach(func() {
				iptablesAdapter.BulkAppendReturnsOnCall(1, errors.New("banana"))
			})

			It("returns an error", func() {
				err := proxyRedirect.Apply(containerNetNamespace)
				Expect(err).To(MatchError("do in container: appending to proxy chain: banana"))
			})
		})
	})
})
