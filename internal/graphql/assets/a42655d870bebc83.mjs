/* esm.sh - graphql@17.1.0/utilities/concatAST */
import{Kind as t}from"./374bb6ac2eb6f198.mjs";function f(o){let n=[];for(let i of o)n.push(...i.definitions);return{kind:t.DOCUMENT,definitions:n}}export{f as concatAST};
