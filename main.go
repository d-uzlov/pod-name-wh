package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

func mutatePod(w http.ResponseWriter, r *http.Request) {
	fmt.Println("got request")

	var admissionReview admissionv1.AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&admissionReview); err != nil {
		http.Error(w, fmt.Sprintf("error decoding request: %v", err), http.StatusBadRequest)
		return
	}

	podResource := metav1.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	if admissionReview.Request.Resource != podResource {
		http.Error(w, "unexpected resource type", http.StatusBadRequest)
		return
	}

	pod := corev1.Pod{}
	if err := json.Unmarshal(admissionReview.Request.Object.Raw, &pod); err != nil {
		http.Error(w, fmt.Sprintf("error unmarshaling pod: %v", err), http.StatusBadRequest)
		return
	}

	if pod.GenerateName == "" {
		fmt.Println("GenerateName is empty", pod.GenerateName, pod.Name, pod.Namespace)
		http.Error(w, "GenerateName is empty", http.StatusBadRequest)
		return
	}
	nodeName := extractNodeName(&pod)
	if nodeName == "" {
		fmt.Println("node not assigned", pod.GenerateName, pod.Name, pod.Namespace)
		http.Error(w, "node not assigned", http.StatusBadRequest)
		return
	}

	// Sanitize node name for use in pod name (DNS-1123 subdomain)
	sanitizedNodeName := strings.ReplaceAll(nodeName, "_", "-")
	newPodName := pod.GenerateName + sanitizedNodeName

	if errs := validation.IsDNS1123Subdomain(newPodName); len(errs) > 0 {
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
	fmt.Println("patching", patch)

	patchBytes, _ := json.Marshal(patch)

	admissionReview.Response = &admissionv1.AdmissionResponse{
		UID:     admissionReview.Request.UID,
		Allowed: true,
		Patch:   patchBytes,
		PatchType: func() *admissionv1.PatchType {
			pt := admissionv1.PatchTypeJSONPatch
			return &pt
		}(),
	}

	json.NewEncoder(w).Encode(admissionReview)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate-pod", mutatePod)
	server := &http.Server{
		Addr:    ":8443",
		Handler: mux,
	}

	go func() {
		err := server.ListenAndServeTLS("/certs/tls.crt", "/certs/tls.key")
		if !errors.Is(err, http.ErrServerClosed) {
			fmt.Println("HTTP server error", err)
		}
		fmt.Println("Stopped serving new connections.")
	}()

	<-ctx.Done()

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Println("HTTP shutdown error", err)
	}
	fmt.Println("Graceful shutdown complete.")
}
