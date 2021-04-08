package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/utilitywarehouse/kube-policy-semaphore/calico"
	"github.com/utilitywarehouse/kube-policy-semaphore/kube"
	"github.com/utilitywarehouse/kube-policy-semaphore/log"
	"k8s.io/client-go/kubernetes"
)

const (
	labelManagedBy         = "managed-by"
	keyManagedBy           = "calico-global-network-sync-operator"
	labelRemoteClusterName = "remote-cluster-name"
	labelNetSetName        = "name"
	labelNetSetNamespace   = "namespace"
)

var (
	flagKubeConfigPath           = flag.String("local-kube-config", getEnv("LOCAL_KUBE_CONFIG", ""), "Path of the local kube cluster config file, if not provided the app will try to get in cluster config")
	flagTargetKubeConfigPath     = flag.String("target-kube-config", getEnv("TARGET_KUBE_CONFIG", ""), "(Required) Path of the target cluster kube config file to add wg peers from")
	flagLabelSelector            = flag.String("label-selector", getEnv("LABEL_SELECTOR", "uw.systems/networksets=true"), "Label of pods to watch and create/update network sets.")
	flagNetworkSetNameAnnotation = flag.String("networkset-name-annotation", getEnv("NS_NAME_ANNOTATION", "uw.systems/networkset-name"), "Pod annotation with the name of the set the pod belong to")
	flagLogLevel                 = flag.String("log-level", getEnv("LOG_LEVEL", "info"), "Log level")
	flagRemoteAPIURL             = flag.String("remote-api-url", getEnv("REMOTE_API_URL", ""), "Remote Kubernetes API server URL")
	flagRemoteCAURL              = flag.String("remote-ca-url", getEnv("REMOTE_CA_URL", ""), "Remote Kubernetes CA certificate URL")
	flagRemoteSATokenPath        = flag.String("remote-sa-token-path", getEnv("REMOTE_SERVICE_ACCOUNT_TOKEN_PATH", ""), "Remote Kubernetes cluster token path")
	flagFullStoreResyncPeriod    = flag.Duration("full-store-resync-period", 60*time.Minute, "Frequency to perform a full network set store resync from cache to calico GlocalNetworkPolicies")
	flagPodResyncPeriod          = flag.Duration("pod-resync-period", 60*time.Minute, "Pod watcher cache resync period")
	flagSetsPrefix               = flag.String("sets-prefix", getEnv("SETS_PREFIX", ""), "(required) A prefix used when creating network sets, needed in case of multiple sync instances for different clusters.")

	saToken  = os.Getenv("WS_REMOTE_SERVICE_ACCOUNT_TOKEN")
	bearerRe = regexp.MustCompile(`[A-Z|a-z0-9\-\._~\+\/]+=*`)
)

func usage() {
	flag.Usage()
	os.Exit(1)
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return defaultValue
	}
	return value
}

func main() {
	flag.Parse()
	log.InitLogger("kube-policy-semaphore", *flagLogLevel)
	if *flagSetsPrefix == "" {
		log.Logger.Error("Must specify non-empty prefix for the created globalnetworksets")
		usage()
	}
	if *flagRemoteSATokenPath != "" {
		data, err := ioutil.ReadFile(*flagRemoteSATokenPath)
		if err != nil {
			fmt.Printf("Cannot read file: %s", *flagRemoteSATokenPath)
			os.Exit(1)
		}
		saToken = string(data)
	}

	if saToken != "" {
		saToken = strings.TrimSuffix(saToken, "\n")
		if !bearerRe.Match([]byte(saToken)) {
			log.Logger.Error(
				"The provided token does not match regex",
				"regex", bearerRe.String)
			os.Exit(1)
		}
	}

	homeCalicoClient, err := calico.NewClient(*flagKubeConfigPath)
	if err != nil {
		log.Logger.Error(
			"cannot create kube client for homecluster",
			"err", err,
		)
		usage()
	}
	var remoteClient *kubernetes.Clientset
	if *flagTargetKubeConfigPath != "" {
		remoteClient, err = kube.ClientFromConfig(*flagTargetKubeConfigPath)
	} else {
		remoteClient, err = kube.Client(saToken, *flagRemoteAPIURL, *flagRemoteCAURL)
	}
	if err != nil {
		log.Logger.Error(
			"cannot create kube client for remotecluster",
			"err", err,
		)
		usage()
	}

	r := newRunner(
		homeCalicoClient,
		remoteClient,
		*flagSetsPrefix,
		*flagLabelSelector,
		*flagNetworkSetNameAnnotation,
		*flagFullStoreResyncPeriod,
		*flagPodResyncPeriod,
	)
	if err := r.Start(); err != nil {
		log.Logger.Error("Failed to start runner", "err", err)
		os.Exit(1)
	}
	go r.Run()

	sm := http.NewServeMux()
	sm.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		if r.Healthy() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	})
	sm.Handle("/metrics", promhttp.Handler())
	go func() {
		log.Logger.Error("Listen and Serve", "err", http.ListenAndServe(":8080", sm))
	}()
	quit := make(chan os.Signal, 1)
	for {
		select {
		case <-quit:
			log.Logger.Info("Quitting")
		}
	}
	r.Stop()
}
