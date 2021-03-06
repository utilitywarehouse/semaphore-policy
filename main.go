package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/utilitywarehouse/semaphore-policy/calico"
	"github.com/utilitywarehouse/semaphore-policy/kube"
	"github.com/utilitywarehouse/semaphore-policy/log"
	"k8s.io/client-go/kubernetes"
)

const (
	labelManagedBy       = "managed-by"
	valueManagedBy       = "semaphore-policy"
	labelNetSetCluster   = "policy.semaphore.uw.io/cluster"
	labelNetSetName      = "policy.semaphore.uw.io/name"
	labelNetSetNamespace = "policy.semaphore.uw.io/namespace"
)

var (
	flagKubeConfigPath       = flag.String("local-kube-config", getEnv("SP_LOCAL_KUBE_CONFIG", ""), "Path of the local kube cluster config file, if not provided the app will try to get in cluster config")
	flagTargetKubeConfigPath = flag.String("target-kube-config", getEnv("SP_TARGET_KUBE_CONFIG", ""), "(Required) Path of the target cluster kube config file to watch pods")
	flagLogLevel             = flag.String("log-level", getEnv("SP_LOG_LEVEL", "info"), "Log level")
	flagRemoteAPIURL         = flag.String("remote-api-url", getEnv("SP_REMOTE_API_URL", ""), "Remote Kubernetes API server URL")
	flagRemoteCAURL          = flag.String("remote-ca-url", getEnv("SP_REMOTE_CA_URL", ""), "Remote Kubernetes CA certificate URL")
	flagRemoteSATokenPath    = flag.String("remote-sa-token-path", getEnv("SP_REMOTE_SERVICE_ACCOUNT_TOKEN_PATH", ""), "Remote Kubernetes cluster token path")
	flagPodResyncPeriod      = flag.Duration("pod-resync-period", 0, "Pod watcher cache resync period. Disabled by default")
	flagTargetCluster        = flag.String("target-cluster-name", getEnv("SP_TARGET_CLUSTER_NAME", ""), "(required) The name of the cluster from which pods are synced as networksets. It will also be used as a prefix used when creating network sets.")

	saToken  = os.Getenv("SP_REMOTE_SERVICE_ACCOUNT_TOKEN")
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
	log.InitLogger("semaphore-policy", *flagLogLevel)
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

	homeCalicoClient, err := calico.ClientFromConfig(*flagKubeConfigPath)
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
		*flagPodResyncPeriod,
	)
	if err := r.Start(); err != nil {
		log.Logger.Error("Failed to start runner", "err", err)
		os.Exit(1)
	}

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
