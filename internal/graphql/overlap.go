package graphql

import (
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/validator"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

// overlapBudget is the largest overlapCost that the
// OverlappingFieldsCanBeMerged validation rule may run on. The rule
// compares every pair of fields that share a response name, so its work
// grows with the square of the repeat count: 15000 copies of one field
// (within the parser token limit) took about 11s. Measured on every query
// shape tried, one cost unit is at most about 125ns, so the budget keeps
// the rule near 100ms. The GraphiQL introspection query costs about 650.
const overlapBudget = 1_000_000

// validationRules returns the gqlparser default rules with
// OverlappingFieldsCanBeMerged replaced by a version that checks
// overlapBudget first. The rule keeps its name, so it runs in the same
// position and its errors carry the same rule name.
func validationRules() *rules.Rules {
	r := rules.NewDefaultRules()
	r.ReplaceRule(rules.OverlappingFieldsCanBeMergedRule.Name,
		budgetedOverlapRule(overlapBudget))
	return r
}

// budgetedOverlapRule returns a rule that computes overlapCost of the
// document at the first operation or fragment, and either reports one
// error (cost over budget) or runs the gqlparser rule in its own walk of
// the document.
func budgetedOverlapRule(budget int) validator.RuleFunc {
	return func(observers *validator.Events, addError validator.AddErrFunc) {
		checked := false
		check := func(walker *validator.Walker) {
			if checked {
				return
			}
			checked = true
			if cost := overlapCost(walker.Document); cost > budget {
				addError(validator.Message(
					"too many fields or fragment spreads share a response name: overlap cost %d exceeds %d",
					cost, budget))
				return
			}
			inner := &validator.Events{}
			rules.OverlappingFieldsCanBeMergedRule.RuleFunc(inner, addError)
			validator.Walk(walker.Schema, walker.Document, inner)
		}
		observers.OnOperation(func(walker *validator.Walker, _ *ast.OperationDefinition) {
			check(walker)
		})
		observers.OnFragment(func(walker *validator.Walker, _ *ast.FragmentDefinition) {
			check(walker)
		})
	}
}

// responseNameCost accumulates the fields that share one response name.
type responseNameCost struct {
	count  int // fields (or spreads of one fragment)
	weight int // sum of 1 + arguments + argument value nodes per field
	args   int // sum of arguments per field
}

// overlapCost returns an upper estimate of the comparisons that
// OverlappingFieldsCanBeMerged makes for doc. The rule compares only
// fields with the same response name, so the estimate groups every field
// of the document (operations and fragment definitions, inline fragments
// included) by response name, and every fragment spread by fragment name.
// For each group it adds count*weight: each field meets at most count
// others, and a comparison costs about the size of its arguments. It also
// adds args squared, because the rule matches argument lists with a nested
// loop. Grouping by name alone ignores that fields under differently named
// parents are never compared, so the estimate can only be too high.
func overlapCost(doc *ast.QueryDocument) int {
	groups := map[string]*responseNameCost{}
	add := func(key string, weight, args int) {
		g := groups[key]
		if g == nil {
			g = &responseNameCost{}
			groups[key] = g
		}
		g.count++
		g.weight += weight
		g.args += args
	}
	var walk func(ast.SelectionSet)
	walk = func(set ast.SelectionSet) {
		for _, sel := range set {
			switch sel := sel.(type) {
			case *ast.Field:
				name := sel.Alias
				if name == "" {
					name = sel.Name
				}
				weight := 1 + len(sel.Arguments)
				for _, arg := range sel.Arguments {
					weight += valueNodes(arg.Value)
				}
				add(name, weight, len(sel.Arguments))
				walk(sel.SelectionSet)
			case *ast.InlineFragment:
				walk(sel.SelectionSet)
			case *ast.FragmentSpread:
				// "..." cannot start a field name, so the keys never clash.
				add("..."+sel.Name, 1, 0)
			}
		}
	}
	for _, op := range doc.Operations {
		walk(op.SelectionSet)
	}
	for _, frag := range doc.Fragments {
		walk(frag.SelectionSet)
	}
	total := 0
	for _, g := range groups {
		total += g.count*g.weight + g.args*g.args
	}
	return total
}

// valueNodes returns the number of nodes in an argument value tree.
func valueNodes(v *ast.Value) int {
	if v == nil {
		return 0
	}
	n := 1
	for _, child := range v.Children {
		n += valueNodes(child.Value)
	}
	return n
}
