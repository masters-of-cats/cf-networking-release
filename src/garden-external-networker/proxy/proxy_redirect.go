package proxy

import (
	"fmt"
	"lib/rules"

	"github.com/containernetworking/plugins/pkg/ns"
)

//go:generate counterfeiter -o ../fakes/namespaceAdapter.go --fake-name NamespaceAdapter . namespaceAdapter
type namespaceAdapter interface {
	GetNS(netNamespace string) (ns.NetNS, error)
}

type Redirect struct {
	IPTables         rules.IPTablesAdapter
	NamespaceAdapter namespaceAdapter
	ChainName        string
	RedirectCIDR     string
	ProxyPort        int
	ProxyUID         int
}

func NewRedirectRuleSet(chainName string, proxyUID int, redirectCIDR string, proxyPort int) []rules.IPTablesRule {
	return []rules.IPTablesRule{
	// rules.IPTablesRule{"-m", "owner", "--uid-owner", string(strconv.Itoa(proxyUID)), "-j", "RETURN"},
	// rules.IPTablesRule{"-d", redirectCIDR, "-p", "tcp", "-j", "REDIRECT", "--to-ports", string(strconv.Itoa(proxyPort))},

	}
}

func (r *Redirect) Apply(containerNetNamespace string) error {
	netNS, err := r.NamespaceAdapter.GetNS(containerNetNamespace)
	err = netNS.Do(func(_ ns.NetNS) error {
		err = r.IPTables.NewChain("nat", r.ChainName)
		if err != nil {
			return fmt.Errorf("creating chain: %s", err)
		}

		err = r.IPTables.BulkAppend("nat", "OUTPUT", rules.IPTablesRule{"-d", r.RedirectCIDR, "-j", r.ChainName})
		if err != nil {
			return fmt.Errorf("appending to OUTPUT: %s", err)
		}

		err = r.IPTables.BulkAppend("nat", r.ChainName, rules.IPTablesRule{
			"-A", "PROXY_REDIRECT",
			"!", "-d", r.SelfIP,
			"-p", "tcp",
			"-m", "owner", "--uid-owner", r.ProxyUID,
			"-j", "REDIRECT", "--to-port", fmt.Sprintf("%d", r.ProxyPort),
		})
		if err != nil {
			return fmt.Errorf("appending to proxy chain: %s", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("do in container: %s", err)
	}

	return nil
}
