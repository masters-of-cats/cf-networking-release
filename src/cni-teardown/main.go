package main

import (
	"cni-wrapper-plugin/lib"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/silk/lib/adapter"
)

func main() {
	if err := mainWithError(); err != nil {
		log.Fatalf("silk-teardown error: %s", err)
	}
}

func mainWithError() error {
	configFilePath := flag.String("config", "", "path to config file")
	flag.Parse()

	configBytes, err := ioutil.ReadFile(*configFilePath)
	if err != nil {
		panic(err)
	}

	cfg, err := lib.LoadWrapperConfig(configBytes)
	if err != nil {
		panic(err)
		// return fmt.Errorf("load config file: %s", err)
	}

	logger := lager.NewLogger("cni-teardown")
	sink := lager.NewWriterSink(os.Stdout, lager.INFO)
	logger.RegisterSink(sink)

	logger.Info("starting")
	netlinkAdapter := &adapter.NetlinkAdapter{}

	links, err := netlinkAdapter.LinkList()
	if err != nil {
		panic(err)
	}

	for _, link := range links {
		if link.Type() == "ifb" && strings.HasPrefix(link.Attrs().Name, "i") {
			err = netlinkAdapter.LinkDel(link)
			if err != nil {
				panic(err)
			}
		}
	}

	// /var/vcap/data/container-metadata
	containerMetadataDir := filepath.Dir(cfg.Datastore)
	err = os.RemoveAll(containerMetadataDir)
	if err != nil {
		logger.Info("failed-to-remove-datastore-path", lager.Data{"path": containerMetadataDir, "err": err})
	}

	// /var/vcap/data/host-local
	if delegateDataDirPath, ok := cfg.Delegate["dataDir"].(string); ok {
		err = os.RemoveAll(delegateDataDirPath)
		if err != nil {
			logger.Info("failed-to-remove-delegate-datastore-path", lager.Data{"path": delegateDataDirPath, "err": err})
		}
	}

	// /var/vcap/data/silk/store.json
	if delegateDataStorePath, ok := cfg.Delegate["datastore"].(string); ok {
		silkDir := filepath.Dir(delegateDataStorePath)
		err = os.RemoveAll(silkDir)
		if err != nil {
			panic(err)
			logger.Info("failed-to-remove-delegate-data-dir-path", lager.Data{"path": silkDir, "err": err})
		}
	}

	var errList error
	logger.Info("complete")

	return errList
}
