package web

import (
	"fmt"
	"net/http"
	"strings"
)

// bashCompletionScript is the downloadable bash completion script for PeeringDB Plus.
// Security: completion search returns integer IDs only, preventing shell injection
// from entity names containing metacharacters (per research Pitfall 4).
const bashCompletionScript = `#!/bin/bash
# PeeringDB Plus shell completions
# Install: eval "$(curl -s peeringdb-plus.fly.dev/ui/completions/bash)"
# Or save: curl -s peeringdb-plus.fly.dev/ui/completions/bash > ~/.pdb-completions.bash
#          source ~/.pdb-completions.bash

_PDB_HOST="${PDB_HOST:-peeringdb-plus.fly.dev}"

pdb() {
  curl -s "${_PDB_HOST}/ui/$@"
}

_pdb_completions() {
  local cur="${COMP_WORDS[$COMP_CWORD]}"
  local prev="${COMP_WORDS[$COMP_CWORD-1]}"

  case "$prev" in
    asn|net)
      COMPREPLY=($(compgen -W "$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${cur}&type=net" 2>/dev/null)" -- "$cur"))
      ;;
    ix)
      COMPREPLY=($(compgen -W "$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${cur}&type=ix" 2>/dev/null)" -- "$cur"))
      ;;
    fac)
      COMPREPLY=($(compgen -W "$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${cur}&type=fac" 2>/dev/null)" -- "$cur"))
      ;;
    org)
      COMPREPLY=($(compgen -W "$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${cur}&type=org" 2>/dev/null)" -- "$cur"))
      ;;
    pdb)
      COMPREPLY=($(compgen -W "asn ix fac org campus carrier compare" -- "$cur"))
      ;;
  esac
}

complete -F _pdb_completions pdb
`

// zshCompletionScript is the downloadable zsh completion script for PeeringDB Plus.
const zshCompletionScript = `#!/bin/zsh
# PeeringDB Plus shell completions for zsh
# Install: eval "$(curl -s peeringdb-plus.fly.dev/ui/completions/zsh)"
# Or save to a file in your fpath

_PDB_HOST="${PDB_HOST:-peeringdb-plus.fly.dev}"

pdb() {
  curl -s "${_PDB_HOST}/ui/$@"
}

_pdb() {
  local -a subcmds
  subcmds=(asn ix fac org campus carrier compare)

  _arguments \
    '1:entity type:(${subcmds})' \
    '2:identifier:->ident'

  case $state in
    ident)
      local type="${words[2]}"
      local -a completions
      completions=(${(f)"$(curl -sf "${_PDB_HOST}/ui/completions/search?q=${words[3]}&type=${type}" 2>/dev/null)"})
      compadd -a completions
      ;;
  esac
}

compdef _pdb pdb
`

// completionSearchLimit is the maximum number of results per type returned by the
// completion search endpoint.
const completionSearchLimit = 20

// handleCompletionBash serves the bash completion script as plain text.
func (h *Handler) handleCompletionBash(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline")
	fmt.Fprint(w, bashCompletionScript)
}

// handleCompletionZsh serves the zsh completion script as plain text.
func (h *Handler) handleCompletionZsh(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", "inline")
	fmt.Fprint(w, zshCompletionScript)
}

// handleCompletionSearch returns newline-delimited entity IDs matching the query.
// Accepts query params: q (search term, min 2 chars) and type (optional: net, ix, fac, org, campus, carrier).
// Returns integer IDs only to prevent shell injection from entity names (SEC-1).
func (h *Handler) handleCompletionSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	typeFilter := r.URL.Query().Get("type")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	if len(q) < 2 {
		return
	}

	results, err := h.searcher.Search(r.Context(), q)
	if err != nil {
		return
	}

	for _, tr := range results {
		if typeFilter != "" && tr.TypeSlug != typeFilter {
			continue
		}

		count := 0
		for _, hit := range tr.Results {
			if count >= completionSearchLimit {
				break
			}
			id := extractID(hit.DetailURL, tr.TypeSlug)
			if id != "" {
				fmt.Fprintln(w, id)
				count++
			}
		}
	}
}

// extractID extracts the entity identifier from a detail URL.
// For networks (TypeSlug "net"), returns the ASN from "/ui/asn/<asn>".
// For other types, returns the ID from "/ui/<type>/<id>".
func extractID(detailURL, typeSlug string) string {
	var prefix string
	switch typeSlug {
	case "net":
		prefix = "/ui/asn/"
	case "ix", "fac", "org", "campus", "carrier":
		prefix = "/ui/" + typeSlug + "/"
	default:
		return ""
	}
	if id, ok := strings.CutPrefix(detailURL, prefix); ok {
		return id
	}
	return ""
}
