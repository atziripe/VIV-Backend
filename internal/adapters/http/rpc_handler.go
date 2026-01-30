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

	// seguridad básica
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
		context.Background(), // 👈 contexto limpio
		method,
		internalURL,
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		log.Printf("401 error: %v", err)
		http.Error(w, "failed to build request", http.StatusInternalServerError)
		return
	}

	// Reinyectar userID en el contexto (sin chi route ctx sucio)
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
		Body:   respBody,
	}

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

// 👇 WHITELIST CLARA Y SIMPLE
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

	case method == "POST" && path == "/training/resume":
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
		// Esperamos que FlutterFlow mande algo como:
		// body: { "plan_generate": { "checkin_id": "..." } }
		// o body: { "plan_generate": {} }
		if v, ok := body["plan_generate"]; ok {
			// Si viene un map, verificamos checkin_id
			if m, ok := v.(map[string]any); ok {
				if raw, exists := m["checkin_id"]; exists {
					// si viene nil, "", o whitespace → starter plan → {}
					if raw == nil {
						return map[string]any{}
					}
					if s, ok := raw.(string); ok {
						if strings.TrimSpace(s) == "" {
							return map[string]any{}
						}
						// valor válido
						return map[string]any{"checkin_id": strings.TrimSpace(s)}
					}
					// si viene de otro tipo, lo devolvemos tal cual (por si FF manda algo raro)
					return m
				}
				// no existe checkin_id → starter plan
				return map[string]any{}
			}

			// Si FlutterFlow manda directamente un string checkin_id (no recomendado)
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return map[string]any{"checkin_id": strings.TrimSpace(s)}
			}

			// Cualquier otro caso → starter plan
			return map[string]any{}
		}

		// Si no viene plan_generate → starter plan
		return map[string]any{}

	case "/checkins":
		// Esperamos que FlutterFlow mande:
		// body: { "checkins": { ... } }
		v, ok := body["checkins"]
		if !ok || v == nil {
			return map[string]any{}
		}

		m, ok := v.(map[string]any)
		if !ok {
			// Si FlutterFlow manda algo raro, no rompemos: devolvemos empty
			return map[string]any{}
		}

		out := map[string]any{}

		// helper: copia si existe, y si es string -> trim
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
			// otros tipos (bool, number, array, map) se pasan tal cual
			out[key] = raw
		}

		copyTrim("sleep_quality")
		copyTrim("body_status")
		copyTrim("appetite")
		copyTrim("stress_level")
		copyTrim("cycle_start")
		copyTrim("workload_prediction")
		copyTrim("mental_energy")
		copyTrim("week_sessions")

		// Si no vino nada usable, regresa {}
		if len(out) == 0 {
			return map[string]any{}
		}
		return out
	case "/lifestyle-changes":
		// Esperamos:
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

		// según tu schema
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
		// Esperamos:
		// body: { "adjust_plan": { "lifestyle_change_id": "..." } }
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
				// si viene de otro tipo raro, devolvemos el map tal cual
				return m
			}

			// si FF manda directo el string (no ideal)
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
		"pms_symptoms",
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
