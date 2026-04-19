package pdbcompat

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ResponseTooLargeType is the RFC 9457 problem-type URI for pre-flight
// budget-exceeded 413 responses (Phase 71 D-04). Package constant so
// Plan 04's handler wiring and Plan 72's parity tests reference the
// same literal — drift between the wire value and the documented value
// is a silent compatibility break.
const ResponseTooLargeType = "https://peeringdb-plus.fly.dev/errors/response-too-large"

// BudgetExceeded describes a request whose estimated response size
// exceeds the configured PDBPLUS_RESPONSE_MEMORY_LIMIT. Populated by
// CheckBudget; consumed by WriteBudgetProblem (413 writer) and (in
// later plans) structured-log emission in the handler + OTel span
// attributes for the memory-budget counter.
//
// Only MaxRows and BudgetBytes are serialized onto the wire (D-04);
// EstimatedBytes, Count, Entity, and Depth are internal diagnostics
// carried in the struct so callers can log / trace them without a
// second trip through TypicalRowBytes.
type BudgetExceeded struct {
	MaxRows        int    `json:"max_rows"`
	BudgetBytes    int64  `json:"budget_bytes"`
	EstimatedBytes int64  `json:"-"`
	Count          int    `json:"-"`
	Entity         string `json:"-"`
	Depth          int    `json:"-"`
}

// CheckBudget reports whether a request of `count` rows for `entity` at
// `depth` fits under `budgetBytes`. Returns (zero, true) when it does,
// or (populated, false) with diagnostic fields when it does not.
//
// budgetBytes <= 0 disables the check entirely — same semantic as
// PDBPLUS_SYNC_MEMORY_LIMIT=0. This is the documented local-dev escape
// hatch and the reason Phase 68's unbounded limit=0 is safe to expose
// in prod (the budget is the DoS safety net).
//
// The math: perRow = TypicalRowBytes(entity, depth); estimated = count
// × perRow; over budget iff estimated > budgetBytes. The estimate is
// conservative by construction — TypicalRowBytes is the measured mean
// doubled and rounded up (D-03), and count is a precise SELECT COUNT(*)
// against the already-filtered query, so false-positive 413s are rare
// and preferred over OOM.
//
// Overflow: count is an int from SELECT COUNT(*); perRow tops out near
// 10 KiB in the lookup table. count × perRow comfortably fits int64
// until count exceeds ~10^15, which is 10^8× the current PeeringDB
// fleet size. No overflow guard needed in practice.
func CheckBudget(count int, entity string, depth int, budgetBytes int64) (BudgetExceeded, bool) {
	if budgetBytes <= 0 {
		return BudgetExceeded{}, true
	}
	perRow := TypicalRowBytes(entity, depth)
	if perRow <= 0 {
		// Defensive: TypicalRowBytes returns defaultRowSize (>0) for
		// unknown entities, so this branch is unreachable under the
		// current map. Guards against a future map mutation that
		// zeroes a value without also updating this check.
		perRow = defaultRowSize
	}
	estimated := int64(count) * int64(perRow)
	if estimated <= budgetBytes {
		return BudgetExceeded{}, true
	}
	maxRows := int(budgetBytes / int64(perRow))
	return BudgetExceeded{
		MaxRows:        maxRows,
		BudgetBytes:    budgetBytes,
		EstimatedBytes: estimated,
		Count:          count,
		Entity:         entity,
		Depth:          depth,
	}, false
}

// budgetProblemBody is the exact on-the-wire shape per Phase 71 D-04.
// Hand-rolled rather than reusing httperr.ProblemDetail because the
// latter hardcodes Type="about:blank" and lacks the max_rows /
// budget_bytes extension fields. Keeping the custom shape in a local
// struct confines the extension to the pdbcompat surface.
type budgetProblemBody struct {
	Type        string `json:"type"`
	Title       string `json:"title"`
	Status      int    `json:"status"`
	Detail      string `json:"detail"`
	Instance    string `json:"instance,omitempty"`
	MaxRows     int    `json:"max_rows"`
	BudgetBytes int64  `json:"budget_bytes"`
}

// WriteBudgetProblem writes the RFC 9457 413 response described by
// Phase 71 D-04. Does not set Retry-After — the failure is
// request-shape (wrong filters / too-large page), not transient
// resource pressure; retrying the same request would produce the same
// 413. Operators who want to retrieve more rows must narrow their
// filters or page smaller.
//
// `instance` is written through untouched to the `instance` field of
// the problem-detail body when non-empty; callers typically pass
// r.URL.Path.
func WriteBudgetProblem(w http.ResponseWriter, instance string, info BudgetExceeded) {
	body := budgetProblemBody{
		Type:   ResponseTooLargeType,
		Title:  "Response exceeds memory budget",
		Status: http.StatusRequestEntityTooLarge,
		Detail: fmt.Sprintf(
			"Request would return ~%d rows totaling ~%d bytes; limit is %d bytes",
			info.Count, info.EstimatedBytes, info.BudgetBytes,
		),
		Instance:    instance,
		MaxRows:     info.MaxRows,
		BudgetBytes: info.BudgetBytes,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Powered-By", poweredByHeader)
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_ = json.NewEncoder(w).Encode(body)
}
