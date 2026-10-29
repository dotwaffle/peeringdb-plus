package pdbcompat

import (
	"encoding/json"
	"fmt"
	"strings"

	"entgo.io/ent/dialect/sql"
	"github.com/dotwaffle/peeringdb-plus/internal/sqlitesearch"
)

// buildJSONPredicate compares normalized JSON text. Parameters stay bound
// values, including LIKE metacharacters and the whole IN array. Native
// JSON containment and MySQL's JSON ordering remain divergences.
func buildJSONPredicate(col, op, value string) (func(*sql.Selector), error) {
	value = strings.ToLower(sqlitesearch.JSONText(value))
	condition := ""
	switch coerceToCaseInsensitive(op) {
	case "", "iexact":
		condition = "%s = ?"
	case "icontains":
		condition, value = "%s LIKE ? ESCAPE '\\'", "%"+likeEscape(value)+"%"
	case "istartswith":
		condition, value = "%s LIKE ? ESCAPE '\\'", likeEscape(value)+"%"
	case "in":
		if value == "" {
			return nil, errEmptyIn
		}
		parts := strings.Split(value, ",")
		for i := range parts {
			parts[i] = strings.ToLower(sqlitesearch.JSONText(parts[i]))
		}
		encoded, err := json.Marshal(parts)
		if err != nil {
			return nil, fmt.Errorf("marshal JSON filter values: %w", err)
		}
		condition, value = "%s IN (SELECT value FROM json_each(?))", string(encoded)
	default:
		return nil, fmt.Errorf("unsupported JSON text operator %q", op)
	}
	return func(s *sql.Selector) {
		field := s.C(col)
		expr := "pdbplus_search_lower(pdbplus_json_text(CAST(" + field + " AS TEXT)))"
		s.Where(sql.And(sql.NotNull(field), sql.ExprP(fmt.Sprintf(condition, expr), value)))
	}, nil
}
