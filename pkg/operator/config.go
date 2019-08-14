package operator

import (
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"path/filepath"
	"time"

	"github.com/appscode/kubed/pkg/eventer"
	rbin "github.com/appscode/kubed/pkg/recyclebin"
	resource_indexer "github.com/appscode/kubed/pkg/registry/resource"
	"github.com/appscode/kubed/pkg/syncer"
	srch_cs "github.com/appscode/searchlight/client/clientset/versioned"
	searchlightinformers "github.com/appscode/searchlight/client/informers/externalversions"
	vcs "github.com/appscode/voyager/client/clientset/versioned"
	voyagerinformers "github.com/appscode/voyager/client/informers/externalversions"
	prominformers "github.com/coreos/prometheus-operator/pkg/client/informers/externalversions"
	pcm "github.com/coreos/prometheus-operator/pkg/client/versioned"
	"github.com/robfig/cron/v3"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"kmodules.xyz/client-go/discovery"
	"kmodules.xyz/client-go/tools/fsnotify"
	kcs "kubedb.dev/apimachinery/client/clientset/versioned"
	kubedbinformers "kubedb.dev/apimachinery/client/informers/externalversions"
	scs "stash.appscode.dev/stash/client/clientset/versioned"
	stashinformers "stash.appscode.dev/stash/client/informers/externalversions"
)

type Config struct {
	ScratchDir        string
	ConfigPath        string
	OperatorNamespace string
	ResyncPeriod      time.Duration
	Test              bool
}

type OperatorConfig struct {
	Config

	ClientConfig      *rest.Config
	DynamicClient dynamic.Interface
	KubeClient        kubernetes.Interface
	VoyagerClient     vcs.Interface
	SearchlightClient srch_cs.Interface
	StashClient       scs.Interface
	KubeDBClient      kcs.Interface
	PromClient        pcm.Interface
}

func NewOperatorConfig(clientConfig *rest.Config) *OperatorConfig {
	return &OperatorConfig{
		ClientConfig: clientConfig,
	}
}

func (c *OperatorConfig) New() (*Operator, error) {
	if err := discovery.IsDefaultSupportedVersion(c.KubeClient); err != nil {
		return nil, err
	}

	op := &Operator{
		Config:            c.Config,
		ClientConfig:      c.ClientConfig,
		DynamicClient : c.DynamicClient,
		KubeClient:        c.KubeClient,
		VoyagerClient:     c.VoyagerClient,
		SearchlightClient: c.SearchlightClient,
		StashClient:       c.StashClient,
		KubeDBClient:      c.KubeDBClient,
		PromClient:        c.PromClient,
	}

	op.recorder = eventer.NewEventRecorder(op.KubeClient, "kubed")
	op.trashCan = &rbin.RecycleBin{}
	op.eventProcessor = &eventer.EventForwarder{Client: op.KubeClient.Discovery()}
	op.configSyncer = syncer.New(op.KubeClient, op.recorder)

	op.cron = cron.New()
	op.cron.Start()

	// Enable full text indexing to have search feature
	indexDir := filepath.Join(c.ScratchDir, "indices")
	op.Indexer = resource_indexer.NewIndexer(indexDir)

	op.Configure()

	op.watcher = &fsnotify.Watcher{
		WatchDir: filepath.Dir(c.ConfigPath),
		Reload:   op.Configure,
	}

	// ---------------------------
	op.Factory = dynamicinformer.NewDynamicSharedInformerFactory(op.DynamicClient, c.ResyncPeriod)

	op.kubeInformerFactory = informers.NewSharedInformerFactory(op.KubeClient, c.ResyncPeriod)
	op.voyagerInformerFactory = voyagerinformers.NewSharedInformerFactory(op.VoyagerClient, c.ResyncPeriod)
	op.stashInformerFactory = stashinformers.NewSharedInformerFactory(op.StashClient, c.ResyncPeriod)
	op.searchlightInformerFactory = searchlightinformers.NewSharedInformerFactory(op.SearchlightClient, c.ResyncPeriod)
	op.kubedbInformerFactory = kubedbinformers.NewSharedInformerFactory(op.KubeDBClient, c.ResyncPeriod)
	op.promInformerFactory = prominformers.NewSharedInformerFactory(op.PromClient, c.ResyncPeriod)
	// ---------------------------
	op.setupWorkloadInformers()
	op.setupNetworkInformers()
	op.setupConfigInformers()
	op.setupRBACInformers()
	op.setupCoreInformers()
	op.setupEventInformers()
	op.setupCertificateInformers()
	op.setupStorageInformers()
	// ---------------------------
	op.setupVoyagerInformers()
	op.setupStashInformers()
	op.setupSearchlightInformers()
	op.setupKubeDBInformers()
	op.setupPrometheusInformers()
	// ---------------------------

	if err := op.Configure(); err != nil {
		return nil, err
	}
	return op, nil
}
