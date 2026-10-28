package pdbcompat

import (
	"entgo.io/ent/dialect/sql"
	"github.com/dotwaffle/peeringdb-plus/internal/sqlitesearch"
)

// nameSearchPhrase combines phrase and phrase-prefix clauses. The name
// analyzer has one keyword token. Other fields use word tokens, so a
// phrase can start at any token and its final token can be a prefix.
func nameSearchPhrase(c nameSearchColumn, cols []string, clean string) *sql.Predicate {
	if clean == "" {
		return sql.False()
	}
	tokens := sqlitesearch.Tokens(clean)
	predicates := make([]*sql.Predicate, 0, len(cols))
	for _, col := range cols {
		if col == "name" {
			predicates = append(predicates, sql.ExprP("instr(pdbplus_search_lower(coalesce("+c(col)+", '')), ?) = 1", clean))
		} else if tokens != "" {
			predicates = append(predicates, sql.ExprP("instr(' ' || pdbplus_search_tokens(coalesce("+c(col)+", '')), ?) > 0", " "+tokens))
		}
	}
	return sql.Or(predicates...)
}
