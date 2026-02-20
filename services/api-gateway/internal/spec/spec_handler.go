// services/api-gateway/internal/spec/spec_handler.go
package spec

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/Medex81/ProjectorBackend/pkg/common/api"
)

type SpecHandler struct {
	mu         sync.RWMutex
	specs      map[string]*api.ServiceAPISpec
	mergedSpec *api.ServiceAPISpec
	etag       string
}

func NewSpecHandler() *SpecHandler {
	return &SpecHandler{
		specs: make(map[string]*api.ServiceAPISpec),
		mergedSpec: &api.ServiceAPISpec{
			ServiceName: "merged",
			Version:     "1.0.0",
			Spec: api.OpenAPI{
				OpenAPI: "3.0.0",
				Info: api.Info{
					Title:       "ProjectorBackend API",
					Description: "Merged API specification for all services",
					Version:     "1.0.0",
					Contact: api.Contact{
						Name:  "ProjectorBackend",
						Email: "support@projector.com",
						URL:   "https://github.com/Medex81/ProjectorBackend",
					},
				},
				Paths:      make(api.Paths),
				Components: api.Components{Schemas: make(map[string]*api.Schema)},
				Tags:       []api.Tag{},
			},
		},
	}
}

func (h *SpecHandler) UpdateSpec(spec *api.ServiceAPISpec) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Store individual spec
	h.specs[spec.ServiceName] = spec

	// Rebuild merged spec
	h.mergeSpecs()
	h.updateETag()
}

func (h *SpecHandler) GetSpec() *api.ServiceAPISpec {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.mergedSpec
}

func (h *SpecHandler) GetServiceSpec(serviceName string) *api.ServiceAPISpec {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.specs[serviceName]
}

func (h *SpecHandler) GetETag() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.etag
}

func (h *SpecHandler) RemoveSpec(serviceName string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	delete(h.specs, serviceName)
	h.mergeSpecs()
	h.updateETag()
}

func (h *SpecHandler) mergeSpecs() {
	// Reset merged spec
	h.mergedSpec.Spec.Paths = make(api.Paths)
	h.mergedSpec.Spec.Components.Schemas = make(map[string]*api.Schema)
	h.mergedSpec.Spec.Tags = []api.Tag{}

	// Merge all specs
	for _, spec := range h.specs {
		h.mergedSpec.Merge(spec)
	}
}

func (h *SpecHandler) updateETag() {
	data, _ := json.Marshal(h.mergedSpec)
	hash := md5.Sum(data)
	h.etag = hex.EncodeToString(hash[:])
}

func (h *SpecHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	spec := h.GetSpec()

	// Check ETag
	if r.Header.Get("If-None-Match") == h.GetETag() {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("ETag", h.GetETag())
	w.Header().Set("Cache-Control", "public, max-age=3600")

	json.NewEncoder(w).Encode(spec)
}
