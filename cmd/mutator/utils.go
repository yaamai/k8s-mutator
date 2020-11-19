package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/golang/glog"
	"io/ioutil"
	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"net/http"
	"os"
)

var (
	runtimeScheme = runtime.NewScheme()
	codecs        = serializer.NewCodecFactory(runtimeScheme)
	deserializer  = codecs.UniversalDeserializer()
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

func AddNonExistentPathPatch(patches []JsonPatch, pod *corev1.Pod) []JsonPatch {
	if pod.Spec.InitContainers == nil && hasPatchByPathPrefix(patches, "/spec/initContainers/") {
		glog.Error("Found non existent path patch")
		patches = append([]JsonPatch{JsonPatch{Op: "add", Path: "/spec/initContainers", Value: []interface{}{}}}, patches...)
	}
	if pod.Spec.Containers == nil && hasPatchByPathPrefix(patches, "/spec/containers/") {
		patches = append([]JsonPatch{JsonPatch{Op: "add", Path: "/spec/containers", Value: []interface{}{}}}, patches...)
	}
	if pod.Spec.Volumes == nil && hasPatchByPathPrefix(patches, "/spec/volumes/") {
		patches = append([]JsonPatch{JsonPatch{Op: "add", Path: "/spec/volumes", Value: []interface{}{}}}, patches...)
	}

	return patches
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

func respJson(w http.ResponseWriter, data interface{}) {
	resp, err := json.Marshal(data)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not encode response: %v", err), http.StatusInternalServerError)
	}

	if _, err := w.Write(resp); err != nil {
		http.Error(w, fmt.Sprintf("could not write response: %v", err), http.StatusInternalServerError)
	}
}

func respAdmissionReview(w http.ResponseWriter, admissionReview *v1beta1.AdmissionReview, admissionResponse *v1beta1.AdmissionResponse) {
	admissionReview.Response = admissionResponse
	admissionReview.Response.UID = admissionReview.Request.UID
	respJson(w, admissionReview)
}

func respErrorAdmissionReview(w http.ResponseWriter, admissionReview *v1beta1.AdmissionReview, err error) {
	resp := &v1beta1.AdmissionResponse{
		Result: &metav1.Status{
			Message: err.Error(),
		},
	}
	respAdmissionReview(w, admissionReview, resp)
}

func respPassthroughAdmissionReview(w http.ResponseWriter, admissionReview *v1beta1.AdmissionReview, err error) {
	resp := &v1beta1.AdmissionResponse{
		Allowed: true,
	}
	respAdmissionReview(w, admissionReview, resp)
}
