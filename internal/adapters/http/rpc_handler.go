package http

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	"github.com/getsentry/sentry-go"
)

type RPCHandler struct {
	target http.Handler
}

func NewRPCHandler(target http.Handler) *RPCHandler {
	return &RPCHandler{target: target}
}

type rpcRequest struct {
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Query  map[string]string `json:"query"`
	Body   map[string]any    `json:"body"`
}

type rpcResponse struct {
	Status int `json:"status"`
	Body   any `json:"body"`
}

func (h *RPCHandler) Handle(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	path := normalizePath(req.Path)

	if method == "" || path == "" {
		http.Error(w, "method and path required", http.StatusBadRequest)
		return
	}

	if !strings.HasPrefix(path, "/") {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if strings.HasPrefix(path, "/rpc") {
		http.Error(w, "rpc recursion not allowed", http.StatusBadRequest)
		return
	}

	if !isAllowedRoute(method, path) {
		http.Error(w, "route not allowed", http.StatusForbidden)
		return
	}

	// query
	q := url.Values{}
	for k, v := range req.Query {
		q.Set(k, v)
	}

	internalURL := path
	if qs := q.Encode(); qs != "" {
		internalURL += "?" + qs
	}

	var bodyBytes []byte
	if method != http.MethodGet && method != http.MethodHead {
		sub := selectSubBody(path, req.Body)
		if path == "/onboarding" {
			if subMap, ok := sub.(map[string]any); ok {
				sub = normalizeOnboardingBody(subMap)
			}
		}
		bodyBytes, _ = json.Marshal(sub)
	}

	log.Printf("[rpc] method=%s path=%s body=%s", method, path, string(bodyBytes))
	userID, ok := UserIDFromContext(r.Context())
	if !ok || userID == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	internalReq, err := http.NewRequestWithContext(
		context.Background(),
		method,
		internalURL,
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		log.Printf("401 error: %v", err)
		if hub := sentry.GetHubFromContext(r.Context()); hub != nil {
			hub.Scope().SetTag("endpoint", r.URL.Path)
			hub.Scope().SetTag("method", r.Method)
			hub.CaptureException(err)
		}
		http.Error(w, "failed to build request", http.StatusInternalServerError)
		return
	}

	internalReq = internalReq.WithContext(WithUserID(internalReq.Context(), userID))
	internalReq.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.target.ServeHTTP(rec, internalReq)

	var respBody any
	if len(rec.Body.Bytes()) > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &respBody)
	}

	out := rpcResponse{
		Status: rec.Code,
		Body:   json.RawMessage(rec.Body.Bytes()),
	}

	//log.Printf("[rpc] upstream status=%d body=%s", rec.Code, json.RawMessage(rec.Body.Bytes()))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

// WHITELIST
func isAllowedRoute(method, path string) bool {
	switch {
	case method == "GET" && path == "/me":
		return true

	case method == "POST" && path == "/onboarding":
		return true

	case method == "POST" && path == "/checkins":
		return true
	case method == "GET" && path == "/checkins/latest":
		return true
	case method == "GET" && path == "/checkins/status":
		return true

	case method == "POST" && path == "/lifestyle-changes":
		return true
	case method == "GET" && path == "/lifestyle-changes":
		return true

	case method == "POST" && path == "/plans/generate":
		return true
	case method == "POST" && path == "/plans/adjust":
		return true
	case method == "GET" && path == "/plans/current":
		return true
	case method == "GET" && strings.HasPrefix(path, "/plans/"):
		return true
	case method == "POST" && path == "/plans/phase-feedback":
		return true

	case method == "POST" && path == "/training/resume":
		return true
	case method == "POST" && path == "/training/generate":
		return true
	case method == "GET" && path == "/training/generate/status":
		return true
	case method == "POST" && path == "/training/validate-arrangement":
		return true
	case method == "POST" && path == "/training/save-arrangement":
		return true
	case method == "GET" && strings.HasPrefix(path, "/training/"):
		return true
	case method == "POST" && strings.HasPrefix(path, "/training/"):
		return true
	case method == "GET" && strings.HasPrefix(path, "/nutrition/"):
		return true
	case method == "POST" && strings.HasPrefix(path, "/nutrition/"):
		return true
	case method == "GET" && path == "/recovery/today":
		return true
	}
	return false
}

func selectSubBody(path string, body map[string]any) any {
	switch path {

	case "/onboarding":
		if v, ok := body["onboarding"]; ok {
			return v
		}
		return map[string]any{}

	case "/plans/generate":
		// body: { "plan_generate": { "checkin_id": "..." } }
		// o body: { "plan_generate": {} }
		if v, ok := body["plan_generate"]; ok {
			if m, ok := v.(map[string]any); ok {
				if raw, exists := m["checkin_id"]; exists {
					if raw == nil {
						return map[string]any{}
					}
					if s, ok := raw.(string); ok {
						if strings.TrimSpace(s) == "" {
							return map[string]any{}
						}
						return map[string]any{"checkin_id": strings.TrimSpace(s)}
					}
					return m
				}
				return map[string]any{}
			}

			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return map[string]any{"checkin_id": strings.TrimSpace(s)}
			}

			return map[string]any{}
		}

		return map[string]any{}

	case "/plans/phase-feedback":
		v, ok := body["phase_feedback"]
		if !ok || v == nil {
			return map[string]any{}
		}

		m, ok := v.(map[string]any)
		if !ok {
			return map[string]any{}
		}

		out := map[string]any{}

		copyTrim := func(key string) {
			raw, exists := m[key]
			if !exists || raw == nil {
				return
			}
			if s, ok := raw.(string); ok {
				t := strings.TrimSpace(s)
				if t == "" {
					return
				}
				out[key] = t
				return
			}
			out[key] = raw
		}

		copyTrim("plan_id")
		copyTrim("phase")
		copyTrim("hormonal_briefing_resonates")

		if len(out) == 0 {
			return map[string]any{}
		}
		return out

	case "/training/generate":
		if v, ok := body["training_generate"]; ok {
			if m, ok := v.(map[string]any); ok {
				if raw, exists := m["checkin_id"]; exists {
					if raw == nil {
						return map[string]any{}
					}
					if s, ok := raw.(string); ok {
						if strings.TrimSpace(s) == "" {
							return map[string]any{}
						}
						return map[string]any{"checkin_id": strings.TrimSpace(s)}
					}
					return m
				}
				return map[string]any{}
			}

			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return map[string]any{"checkin_id": strings.TrimSpace(s)}
			}

			return map[string]any{}
		}

		return map[string]any{}

	case "/training/complete-day":
		v, ok := body["training_completed"]
		if !ok || v == nil {
			return map[string]any{}
		}

		m, ok := v.(map[string]any)
		if !ok {
			return map[string]any{}
		}

		out := map[string]any{}

		copyTrim := func(key string) {
			raw, exists := m[key]
			if !exists || raw == nil {
				return
			}
			if s, ok := raw.(string); ok {
				t := strings.TrimSpace(s)
				if t == "" {
					return
				}
				out[key] = t
				return
			}
			out[key] = raw
		}

		copyTrim("plan_id")
		copyTrim("weekday")

		if len(out) == 0 {
			return map[string]any{}
		}
		return out

	case "/training/save-arrangement":
		v, ok := body["training_save_arrangement"]
		if !ok || v == nil {
			return map[string]any{}
		}

		m, ok := v.(map[string]any)
		if !ok {
			return map[string]any{}
		}

		out := map[string]any{}

		copyTrim := func(key string) {
			raw, exists := m[key]
			if !exists || raw == nil {
				return
			}
			if s, ok := raw.(string); ok {
				t := strings.TrimSpace(s)
				if t == "" {
					return
				}
				out[key] = t
				return
			}
			out[key] = raw
		}

		copyTrim("training_plan_id")
		copyTrim("days_json")

		if len(out) == 0 {
			return map[string]any{}
		}
		return out

	case "/training/validate-arrangement":
		v, ok := body["training_validate_arrangement"]
		if !ok || v == nil {
			return map[string]any{}
		}

		m, ok := v.(map[string]any)
		if !ok {
			return map[string]any{}
		}

		out := map[string]any{}

		copyTrim := func(key string) {
			raw, exists := m[key]
			if !exists || raw == nil {
				return
			}
			if s, ok := raw.(string); ok {
				t := strings.TrimSpace(s)
				if t == "" {
					return
				}
				out[key] = t
				return
			}
			out[key] = raw
		}

		copyTrim("phase")

		if raw, exists := m["days"]; exists && raw != nil {
			out["days"] = raw
		}

		if len(out) == 0 {
			return map[string]any{}
		}
		return out

	case "/nutrition/meal-selection":
		v, ok := body["meal_selection"]
		if !ok || v == nil {
			return map[string]any{}
		}

		m, ok := v.(map[string]any)
		if !ok {
			return map[string]any{}
		}

		out := map[string]any{}

		copyTrim := func(key string) {
			raw, exists := m[key]
			if !exists || raw == nil {
				return
			}
			if s, ok := raw.(string); ok {
				t := strings.TrimSpace(s)
				if t == "" {
					return
				}
				out[key] = t
				return
			}
			out[key] = raw
		}

		copyTrim("plan_id")
		copyTrim("weekday")
		copyTrim("meal_slot")
		copyTrim("option_index")

		if len(out) == 0 {
			return map[string]any{}
		}
		return out

	case "/checkins":
		// body: { "checkins": { ... } }
		v, ok := body["checkins"]
		if !ok || v == nil {
			return map[string]any{}
		}

		m, ok := v.(map[string]any)
		if !ok {
			return map[string]any{}
		}

		out := map[string]any{}

		copyTrim := func(key string) {
			raw, exists := m[key]
			if !exists || raw == nil {
				return
			}
			if s, ok := raw.(string); ok {
				t := strings.TrimSpace(s)
				if t == "" {
					return
				}
				out[key] = t
				return
			}
			out[key] = raw
		}

		copyTrim("sleep_quality")
		copyTrim("body_status")
		copyTrim("appetite")
		copyTrim("demand")
		copyTrim("cycle_start")
		copyTrim("last_week_feeling")
		copyTrim("pms_symptoms")
		copyTrim("predictability")
		copyTrim("readiness")
		copyTrim("additional_notes")

		if len(out) == 0 {
			return map[string]any{}
		}
		return out
	case "/lifestyle-changes":
		// body: { "change_lifestyle": { "type": "...", "space_train": "...", ... } }
		v, ok := body["change_lifestyle"]
		if !ok || v == nil {
			return map[string]any{}
		}

		m, ok := v.(map[string]any)
		if !ok {
			return map[string]any{}
		}

		out := map[string]any{}

		copyTrim := func(key string) {
			raw, exists := m[key]
			if !exists || raw == nil {
				return
			}
			if s, ok := raw.(string); ok {
				t := strings.TrimSpace(s)
				if t == "" {
					return
				}
				out[key] = t
				return
			}
			out[key] = raw
		}

		copyTrim("type")
		copyTrim("space_train")
		copyTrim("possible_diet")
		copyTrim("energy")
		copyTrim("applies_to")

		if len(out) == 0 {
			return map[string]any{}
		}
		return out

	case "/plans/adjust":
		if v, ok := body["adjust_plan"]; ok {
			if m, ok := v.(map[string]any); ok {
				raw, exists := m["lifestyle_change_id"]
				if !exists || raw == nil {
					return map[string]any{}
				}
				if s, ok := raw.(string); ok {
					t := strings.TrimSpace(s)
					if t == "" {
						return map[string]any{}
					}
					return map[string]any{"lifestyle_change_id": t}
				}
				return m
			}

			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return map[string]any{"lifestyle_change_id": strings.TrimSpace(s)}
			}

			return map[string]any{}
		}
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

func normalizeOnboardingBody(body map[string]any) map[string]any {
	// Campos que en FlutterFlow pueden venir como List<String> (decodea como []any)
	listFieldsToJoin := []string{
		"training_type",
		"trouble_in_life",
		"diet_type",
	}

	for _, key := range listFieldsToJoin {
		v, ok := body[key]
		if !ok || v == nil {
			continue
		}

		// FlutterFlow/JSON → []any
		if rawList, ok := v.([]any); ok {
			items := make([]string, 0, len(rawList))
			for _, it := range rawList {
				if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
					items = append(items, s)
				}
			}
			body[key] = strings.Join(items, ", ")
			continue
		}

		// Si en algún caso ya llega como []string
		if strList, ok := v.([]string); ok {
			items := make([]string, 0, len(strList))
			for _, s := range strList {
				s = strings.TrimSpace(s)
				if s != "" {
					items = append(items, s)
				}
			}
			body[key] = strings.Join(items, ", ")
			continue
		}
	}

	return body
}
