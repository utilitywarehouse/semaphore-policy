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
	labelManagedBy       = "managed-by"
	valueManagedBy       = "kube-policy-semaphore"
	labelNetSetCluster   = "semaphore.uw.systems/cluster"
	labelNetSetName      = "semaphore.uw.systems/name"
	labelNetSetNamespace = "semaphore.uw.systems/namespace"
)

var (
	flagKubeConfigPath        = flag.String("local-kube-config", getEnv("KPS_LOCAL_KUBE_CONFIG", ""), "Path of the local kube cluster config file, if not provided the app will try to get in cluster config")
	flagTargetKubeConfigPath  = flag.String("target-kube-config", getEnv("KPS_TARGET_KUBE_CONFIG", ""), "(Required) Path of the target cluster kube config file to watch pods")
	flagLogLevel              = flag.String("log-level", getEnv("KPS_LOG_LEVEL", "info"), "Log level")
	flagRemoteAPIURL          = flag.String("remote-api-url", getEnv("KPS_REMOTE_API_URL", ""), "Remote Kubernetes API server URL")
	flagRemoteCAURL           = flag.String("remote-ca-url", getEnv("KPS_REMOTE_CA_URL", ""), "Remote Kubernetes CA certificate URL")
	flagRemoteSATokenPath     = flag.String("remote-sa-token-path", getEnv("KPS_REMOTE_SERVICE_ACCOUNT_TOKEN_PATH", ""), "Remote Kubernetes cluster token path")
	flagFullStoreResyncPeriod = flag.Duration("full-store-resync-period", 60*time.Minute, "Frequency to perform a full network set store resync from cache to calico GlocalNetworkPolicies")
	flagPodResyncPeriod       = flag.Duration("pod-resync-period", 60*time.Minute, "Pod watcher cache resync period")
	flagTargetCluster         = flag.String("target-cluster-name", getEnv("KPS_TARGET_CLUSTER_NAME", ""), "(required) The name of the cluster from which pods are synced as networksets. It will also be used as a prefix used when creating network sets.")

	saToken  = os.Getenv("KPS_REMOTE_SERVICE_ACCOUNT_TOKEN")
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
	if *flagTargetCluster == "" {
		log.Logger.Error("Must specify non-empty target cluster naeme for the created globalnetworksets")
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
		*flagTargetCluster,
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
