package api

import (
	"net/http"
	"strings"
)

// apiErrorBody is the stable JSON error envelope for math-model hot paths (SCRUM-27).
type apiErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func writeAPIError(w http.ResponseWriter, status int, errCode, message string) {
	writeJSON(w, status, apiErrorBody{Error: errCode, Message: message})
}

func writeFailureAPIError(w http.ResponseWriter, res batchWriteResult) {
	writeAPIError(w, res.HTTP, classifyWriteError(res), res.Error)
}

func classifyWriteError(res batchWriteResult) string {
	msg := strings.ToLower(res.Error)
	switch {
	case strings.Contains(msg, "writable=false"):
		return "tag_not_writable"
	case strings.Contains(msg, "simulate=true"):
		return "tag_simulated"
	case strings.Contains(msg, "ambiguous"):
		return "ambiguous_tag"
	case strings.Contains(msg, "not found"):
		return "tag_not_found"
	case strings.Contains(msg, "not connected"):
		return "device_not_connected"
	case strings.Contains(msg, "device hub unavailable"):
		return "device_hub_unavailable"
	case strings.Contains(msg, "verify"):
		return "verify_failed"
	case res.HTTP == http.StatusBadRequest:
		return "bad_request"
	case res.HTTP == http.StatusBadGateway:
		return "opc_write_failed"
	case res.HTTP == http.StatusServiceUnavailable:
		return "service_unavailable"
	case res.HTTP == http.StatusConflict:
		return "conflict"
	case res.HTTP == http.StatusForbidden:
		return "forbidden"
	default:
		return "write_failed"
	}
}