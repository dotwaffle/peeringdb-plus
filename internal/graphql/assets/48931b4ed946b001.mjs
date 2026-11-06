/* esm.sh - graphql@17.1.0/validation/rules/KnownFragmentNamesRule */
import{GraphQLError as a}from"./3f07b35bd75fe31a.mjs";function o(r){return{FragmentSpread(n){let e=n.name.value;r.getFragment(e)||r.reportError(new a(`Unknown fragment "${e}".`,{nodes:n.name}))}}}export{o as KnownFragmentNamesRule};
