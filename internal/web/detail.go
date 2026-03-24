package web

import "net/http"

// handleNetworkDetail renders the network detail page for the given ASN.
func (h *Handler) handleNetworkDetail(w http.ResponseWriter, r *http.Request, asnStr string) {
	h.handleNotFound(w, r) // Implemented in Task 2
}

// handleIXDetail renders the IXP detail page for the given ID.
func (h *Handler) handleIXDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleFacilityDetail renders the facility detail page for the given ID.
func (h *Handler) handleFacilityDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleOrgDetail renders the organization detail page for the given ID.
func (h *Handler) handleOrgDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleCampusDetail renders the campus detail page for the given ID.
func (h *Handler) handleCampusDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleCarrierDetail renders the carrier detail page for the given ID.
func (h *Handler) handleCarrierDetail(w http.ResponseWriter, r *http.Request, idStr string) {
	h.handleNotFound(w, r) // TODO: Plan 02
}

// handleFragment dispatches lazy-loaded fragment requests.
// Fragment URLs follow the pattern: {parent_type}/{parent_id}/{relation}
func (h *Handler) handleFragment(w http.ResponseWriter, r *http.Request, path string) {
	h.handleNotFound(w, r) // Implemented in Task 2
}
