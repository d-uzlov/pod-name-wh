package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"meoe.io/daemonset-name-webhook/internal/appconfig"

	slogctx "github.com/veqryn/slog-context"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

func extractNodeName(pod *corev1.Pod) string {
	spec := pod.Spec
	if spec.NodeName != "" {
		return spec.NodeName
	}

	if spec.Affinity == nil {
		return ""
	}
	if spec.Affinity.NodeAffinity == nil {
		return ""
	}
	if spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return ""
	}
	if spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms == nil {
		return ""
	}
	affinityRules := pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms

	for _, affinity := range affinityRules {
		for _, fieldRule := range affinity.MatchFields {
			if fieldRule.Key != "metadata.name" {
				continue
			}
			if fieldRule.Operator != "In" {
				continue
			}
			if len(fieldRule.Values) != 1 {
				continue
			}
			return fieldRule.Values[0]
		}
	}
	return ""
}

func mutatePod(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	logger := slogctx.FromCtx(ctx)

	var admissionReview admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&admissionReview); err != nil {
		logger.Error("error decoding request", "error", err.Error())
		http.Error(w, fmt.Sprintf("error decoding request: %v", err), http.StatusBadRequest)
		return
	}

	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if admissionReview.Request.Resource != podResource {
		logger.Error("unexpected resource type", "type", admissionReview.Request.Resource)
		http.Error(w, "unexpected resource type", http.StatusBadRequest)
		return
	}

	pod := corev1.Pod{}
	err := json.Unmarshal(admissionReview.Request.Object.Raw, &pod)
	if err != nil {
		logger.Error("error unmarshaling pod", "error", err.Error())
		http.Error(w, fmt.Sprintf("error unmarshaling pod: %v", err), http.StatusBadRequest)
		return
	}

	logger = logger.With("namespace", pod.Namespace, "pod", pod.Name, "GenerateName", pod.GenerateName)

	if pod.GenerateName == "" {
		logger.Error("GenerateName is empty")
		http.Error(w, "GenerateName is empty", http.StatusBadRequest)
		return
	}
	nodeName := extractNodeName(&pod)
	if nodeName == "" {
		logger.Error("node not assigned")
		http.Error(w, "node not assigned", http.StatusBadRequest)
		return
	}

	sanitizedNodeName := strings.ReplaceAll(nodeName, "_", "-")
	newPodName := pod.GenerateName + sanitizedNodeName
	logger = logger.With("new-name", newPodName)

	if errs := validation.IsDNS1123Subdomain(newPodName); len(errs) > 0 {
		logger.Error("invalid pod name", "errors", errs)
		http.Error(w, fmt.Sprintf("invalid pod name: %v", errs), http.StatusBadRequest)
		return
	}

	// Modify the pod name
	patch := []map[string]interface{}{
		{
			"op":    "replace",
			"path":  "/metadata/name",
			"value": newPodName,
		},
	}
	logger.Debug("patched", "patch", patch)

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		logger.Error("could not marshal patch", "error", err.Error())
	}

	patchType := admissionv1.PatchTypeJSONPatch
	admissionReview.Response = &admissionv1.AdmissionResponse{
		UID:     admissionReview.Request.UID,
		Allowed: true,
		Patch:   patchBytes,
		PatchType: &patchType,
	}

	err = json.NewEncoder(w).Encode(admissionReview)
	if err != nil {
		logger.Error("could not send http response", "error", err.Error())
	}
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	appConfig := appconfig.ParseConfig()

	logLevel := new(slog.LevelVar)
	logLevel.Set(slog.LevelInfo)
	logHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	logger := slog.New(logHandler)

	logger = logger.With("host", appConfig.Hostname)
	ctx = slogctx.NewCtx(ctx, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate-pod", func(w http.ResponseWriter, r *http.Request) {
		mutatePod(ctx, w, r)
	})
	server := &http.Server{
		Addr:    appConfig.ListenAddress,
		Handler: mux,
	}

	go func() {
		err := server.ListenAndServeTLS("/certs/tls.crt", "/certs/tls.key")
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server error", "error", err.Error())
		}
		logger.Info("Stopped serving new connections")
	}()

	<-ctx.Done()
	logger.Info("caught context cancel")

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	err := server.Shutdown(shutdownCtx)
	if err != nil {
		logger.Error("HTTP shutdown error", "error", err.Error())
	}
	logger.Info("Graceful shutdown complete")
}
