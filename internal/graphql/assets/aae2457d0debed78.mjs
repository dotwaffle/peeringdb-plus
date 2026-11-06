/* esm.sh - graphql@17.1.0/utilities/typeFromAST */
import{Kind as t}from"./374bb6ac2eb6f198.mjs";import{GraphQLList as p,GraphQLNonNull as T}from"./593b35bd868e4b3b.mjs";function i(e,r){switch(r.kind){case t.LIST_TYPE:{let n=i(e,r.type);return n&&new p(n)}case t.NON_NULL_TYPE:{let n=i(e,r.type);return n&&new T(n)}case t.NAMED_TYPE:return e.getType(r.name.value)}}export{i as typeFromAST};
