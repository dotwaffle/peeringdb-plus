package pdbcompat

import (
	"maps"
	"net/url"
	"strings"
)

// selectPrepareForms keeps the winning form of each prepare_query filter
// (PeeringDB 2.84.0 serializers.py:708-758). QueryDict iterates keys in
// first-appearance order. A later form replaces the same emitted key.
// Values stay intact because prepare_query reads the first value.
func selectPrepareForms(rawQuery string, params url.Values, tc TypeConfig) url.Values {
	selected := params
	copied := false
	seen := make(map[string]bool)
	winners := make(map[string]string)
	for pair := range strings.SplitSeq(rawQuery, "&") {
		key, _, _ := strings.Cut(pair, "=")
		key, err := url.QueryUnescape(key)
		if err != nil || seen[key] {
			continue
		}
		seen[key] = true
		if _, present := params[key]; !present {
			continue
		}
		group, ok := prepareFormGroup(tc, key)
		if !ok {
			continue
		}
		if previous, exists := winners[group]; exists {
			if !copied {
				selected = maps.Clone(params)
				copied = true
			}
			delete(selected, previous)
		}
		winners[group] = key
	}
	return selected
}

// prepareFormGroup is the key emitted by get_relation_filters. Different
// fields of one relation remain independent. Excluded input never wins.
func prepareFormGroup(tc TypeConfig, key string) (string, bool) {
	parts := strings.Split(key, "__")
	if len(parts) > 3 || relationFilterExcluded(key) {
		return "", false
	}
	root := parts[0]
	sd, seed := relationSeeds[tc.Name][root]
	eligible := seed && sd.shape != shapeWholeKey || tc.ExactCounts[root]
	if _, ok := lookupWhereisKey(tc.Name, root); ok {
		eligible = true
	}
	if _, ok := lookupCapacityFilter(tc.Name, root); ok {
		eligible = true
	}
	if !eligible {
		return "", false
	}
	group := queryableFieldXL(root)
	if len(parts) == 1 || len(parts) == 2 && metaOperators[parts[1]] {
		return group, true
	}
	return group + "__" + queryableFieldXL(parts[1]), true
}
