package models

import "strings"

// ResolveModelID resolves a model reference to a supported ModelID. It accepts
// either a bare ModelID (e.g. "claude-4-sonnet") or a "provider/model" form
// (e.g. "anthropic/claude-4-sonnet").
func ResolveModelID(ref string) (ModelID, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	if _, ok := SupportedModels[ModelID(ref)]; ok {
		return ModelID(ref), true
	}

	provider, model, found := strings.Cut(ref, "/")
	if !found || provider == "" || model == "" {
		return "", false
	}
	// Fast path: the canonical <provider>.<model> id.
	if _, ok := SupportedModels[ModelID(provider+"."+model)]; ok {
		return ModelID(provider + "." + model), true
	}
	for id, m := range SupportedModels {
		if string(m.Provider) != provider {
			continue
		}
		if string(id) == model || strings.HasSuffix(string(id), "/"+model) || strings.HasSuffix(string(id), "."+model) {
			return id, true
		}
	}
	return "", false
}
