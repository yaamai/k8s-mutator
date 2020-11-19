package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"k8s.io/api/admission/v1beta1"
	"k8s.io/client-go/kubernetes"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

type MutateServer struct {
	port         int
	certFilePath string
	keyFilePath  string
	server       *http.Server
	client       kubernetes.Interface
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

	configs, err := NewMutateConfigListFromKubernetes(s.client, configCondition)
	if err != nil {
		respErrorAdmissionReview(w, admissionReview, err)
		return
	}

	patches := AddNonExistentPathPatch(configs.GetPatch(), pod)
	patchesBytes, err := json.Marshal(patches)
	log.Debug().Str("patch", string(patchesBytes)).Msg("Patch generated")

	resp := &v1beta1.AdmissionResponse{
		Allowed: true,
		Patch:   patchesBytes,
		PatchType: func() *v1beta1.PatchType {
			pt := v1beta1.PatchTypeJSONPatch
			return &pt
		}(),
	}

	respAdmissionReview(w, admissionReview, resp)
}

func (s *MutateServer) initServer() error {
	pair, err := tls.LoadX509KeyPair(s.certFilePath, s.keyFilePath)
	if err != nil {
		return err
	}

	s.server = &http.Server{
		Addr:      fmt.Sprintf(":%v", s.port),
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
	}

	s.client = getKubernetesClient()

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", s.handleMutate)
	s.server.Handler = mux

	return nil
}

func (s *MutateServer) serve() error {
	if err := s.initServer(); err != nil {
		return err
	}

	errChan := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServeTLS("", ""); err != nil {
			errChan <- err
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errChan:
		return err
	case <-signalChan:
	}

	log.Info().Msg("shutting down ...")
	return s.server.Shutdown(context.Background())
}