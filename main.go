package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"encoding/json"
	"github.com/golang/glog"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

type MutateServer struct {
	port         int
	certFilePath string
	keyFilePath  string
	server       *http.Server
}

func (s *MutateServer) InitServer() {
	pair, err := tls.LoadX509KeyPair(s.certFilePath, s.keyFilePath)
	if err != nil {
		glog.Errorf("Failed to load key pair: %v", err)
	}

	s.server = &http.Server{
		Addr:      fmt.Sprintf(":%v", s.port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}
}

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

func (s *MutateServer) handleMutate(w http.ResponseWriter, r *http.Request) {
	glog.Error("handleMutate")
	var body []byte
	if r.Body != nil {
		if data, err := ioutil.ReadAll(r.Body); err == nil {
			body = data
		}
	}
	if len(body) == 0 {
		glog.Error("empty body")
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}

	// verify the content type is accurate
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" {
		glog.Errorf("Content-Type=%s, expect application/json", contentType)
		http.Error(w, "invalid Content-Type, expect `application/json`", http.StatusUnsupportedMediaType)
		return
	}

	ar := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &ar); err != nil {
		glog.Errorf("Can't decode body: %v", err)
	}

	glog.Error(ar)

	req := ar.Request
	var pod corev1.Pod
	if err := json.Unmarshal(req.Object.Raw, &pod); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
	}
	glog.Error(pod)

	ar.Response = &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   []byte(`[{"op": "replace", "path": "/spec/containers/0/image", "value": "nginx:1.2.3"}]`),
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
	ar.Response.UID = ar.Request.UID

	resp, err := json.Marshal(ar)
	if err != nil {
		glog.Errorf("Can't encode response: %v", err)
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}
	glog.Infof("Ready to write reponse ...")
	if _, err := w.Write(resp); err != nil {
		glog.Errorf("Can't write response: %v", err)
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}

}

func (s *MutateServer) serve() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", s.handleMutate)
	s.server.Handler = mux

	return s.server.ListenAndServeTLS("", "")
}

func main() {
	var server MutateServer
	flag.IntVar(&server.port, "port", 8443, "Webhook server port.")
	flag.StringVar(&server.certFilePath, "tlsCertFile", "/etc/webhook/certs/cert.pem", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&server.keyFilePath, "tlsKeyFile", "/etc/webhook/certs/key.pem", "File containing the x509 private key to --tlsCertFile.")
	flag.Parse()

	go func() {
		server.InitServer()
		if err := server.serve(); err != nil {
			glog.Errorf("Failed to listen and serve webhook server: %v", err)
		}
	}()

	// listening OS shutdown singal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	glog.Infof("Got OS shutdown signal, shutting down webhook server gracefully...")
	server.server.Shutdown(context.Background())
}
