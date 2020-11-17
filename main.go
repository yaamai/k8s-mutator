package main

import (
	"context"
	"crypto/tls"
	"errors"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func parseMutateWebhook(r *http.Request) (*v1beta1.AdmissionReview, *corev1.Pod, error) {

	if r.Body == nil {
		return nil, nil, errors.New("invalid request body")
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, nil, errors.New("request body read failed")
	}

	if len(body) == 0 {
		return nil, nil, errors.New("empty request body")
	}

	if r.Header.Get("Content-Type") != "application/json" {
		return nil, nil, errors.New("invalid content type")
	}

	admissionReview := v1beta1.AdmissionReview{}
	if _, _, err := deserializer.Decode(body, nil, &admissionReview); err != nil {
		return nil, nil, errors.New("failed to decode AdmissionReview")
	}

	var pod corev1.Pod
	if err := json.Unmarshal(admissionReview.Request.Object.Raw, &pod); err != nil {
		glog.Errorf("Could not unmarshal raw object: %v", err)
	}

	return &admissionReview, &pod, nil
}

type MutateConfigPatch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value"`
}

type MutateConfig struct {
	Patch []MutateConfigPatch `json:"patch"`
}

func gatherMutateConfig(client kubernetes.Interface, configCondition string) ([]MutateConfig, error) {
	// TODO: support more flexible targetCondition
	//       ex.) labelSelect, multiple, [{"label": ""}], ["a", "b"]

	configMap, err := client.CoreV1().ConfigMaps(corev1.NamespaceDefault).Get(configCondition, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	configs := []MutateConfig{}
	for _, value := range configMap.Data {
		mc := MutateConfig{}
		if err := json.Unmarshal([]byte(value), &mc); err != nil {
			continue
		}

		configs = append(configs, mc)
	}

	return configs, nil
}

func isNeedMutation(pod *corev1.Pod) (string, error) {
	value, ok := pod.Annotations["mutate.example.com/config"]
	if !ok {
		return "", nil
	}

	return value, nil
}

func getKubernetesClient() kubernetes.Interface {
	// construct the path to resolve to `~/.kube/config`
	kubeConfigPath := os.Getenv("KUBECONFIG")

	// create the config from the path
	config, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		glog.Errorf("getClusterConfig: %v", err)
	}

	// generate the client based off of the config
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Errorf("getClusterConfig: %v", err)
	}

	glog.Error("Successfully constructed k8s client")
	return client
}

type MutateServer struct {
	port         int
	certFilePath string
	keyFilePath  string
	server       *http.Server
	client       kubernetes.Interface
}

func (s *MutateServer) initServer() {
	pair, err := tls.LoadX509KeyPair(s.certFilePath, s.keyFilePath)
	if err != nil {
		glog.Errorf("Failed to load key pair: %v", err)
	}

	s.server = &http.Server{
		Addr:      fmt.Sprintf(":%v", s.port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}

	s.client = getKubernetesClient()

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", s.handleMutate)
	s.server.Handler = mux
}

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
)

func respJson(w http.ResponseWriter, data interface{}) {
	resp, err := json.Marshal(data)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}

	if _, err := w.Write(resp); err != nil {
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func respErrorAdmissionReview(w http.ResponseWriter, admissionReview *v1beta1.AdmissionReview, err error) {
	resp := &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
	admissionReview.Response = resp
	admissionReview.Response.UID = admissionReview.Request.UID
	respJson(w, admissionReview)
}

func respPassthroughAdmissionReview(w http.ResponseWriter, admissionReview *v1beta1.AdmissionReview, err error) {
	resp := &v1beta1.AdmissionResponse{
		Allowed: true,
	}
	admissionReview.Response = resp
	admissionReview.Response.UID = admissionReview.Request.UID
	respJson(w, admissionReview)
}

func (s *MutateServer) handleMutate(w http.ResponseWriter, r *http.Request) {
	admissionReview, pod, err := parseMutateWebhook(r)
	if err != nil {
		respErrorAdmissionReview(w, admissionReview, err)
		return
	}

	configCondition, err := isNeedMutation(pod)
	if err != nil {
		respErrorAdmissionReview(w, admissionReview, err)
		return
	}
	if configCondition == "" {
		respPassthroughAdmissionReview(w, admissionReview, err)
		return
	}

	configs, err := gatherMutateConfig(s.client, configCondition)
	if err != nil {
		respErrorAdmissionReview(w, admissionReview, err)
		return
	}

	patches := []MutateConfigPatch{}
	for _, val := range configs {
		patches = append(patches, val.Patch...)
	}
	patchesBytes, err := json.Marshal(patches)
	glog.Error("patch gathered", configs, patches, string(patchesBytes))

	resp := &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchesBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}
	admissionReview.Response = resp
	admissionReview.Response.UID = admissionReview.Request.UID

	respJson(w, admissionReview)
}

func (s *MutateServer) serve() error {
	s.initServer()

	go func() {
		if err := s.server.ListenAndServeTLS("", ""); err != nil {
			glog.Errorf("Failed to listen and serve webhook server: %v", err)
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	glog.Infof("Got OS shutdown signal, shutting down webhook server gracefully...")
	return s.server.Shutdown(context.Background())
}

func main() {
	var server MutateServer
	flag.IntVar(&server.port, "port", 8443, "Webhook server port.")
	flag.StringVar(&server.certFilePath, "tlsCertFile", "/etc/webhook/certs/cert.pem", "File containing the x509 Certificate for HTTPS.")
	flag.StringVar(&server.keyFilePath, "tlsKeyFile", "/etc/webhook/certs/key.pem", "File containing the x509 private key to --tlsCertFile.")
	flag.Parse()

	// client := getKubernetesClient()
	// cms, err := client.CoreV1().ConfigMaps(corev1.NamespaceDefault).List(metav1.ListOptions{})
	// cms, err := client.CoreV1().ConfigMaps("").List(metav1.ListOptions{})
	// glog.Error(cms, err)

	server.serve()
}
