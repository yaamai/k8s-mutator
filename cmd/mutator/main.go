package main

import (
	"flag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	var server MutateServer
	flag.IntVar(&server.port, "port", 8443, "Webhook server port.")
	flag.StringVar(&server.certFilePath, "tlsCertFile", "/etc/certs/cert", "TLS certificate file path")
	flag.StringVar(&server.keyFilePath, "tlsKeyFile", "/etc/certs/key", "TLS key file path")

	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", os.Getenv("KUBECONFIG"), "absolute path to the kubeconfig file")
	}
	flag.Parse()

	err := server.serve(kubeconfig)
	if err != nil {
		log.Error().Err(err).Msg("cannot start server")
		return
	}
}
