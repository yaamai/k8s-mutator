package main

import (
	"flag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
)

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	var server MutateServer
	flag.IntVar(&server.port, "port", 8443, "Webhook server port.")
	flag.StringVar(&server.certFilePath, "tlsCertFile", "/etc/certs/cert.pem", "TLS certificate file path")
	flag.StringVar(&server.keyFilePath, "tlsKeyFile", "/etc/certs/key.pem", "TLS key file path")
	flag.Parse()

	server.serve()
}
