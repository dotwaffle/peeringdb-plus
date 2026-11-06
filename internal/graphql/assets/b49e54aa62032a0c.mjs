/* esm.sh - graphql@17.1.0/utilities/getOperationAST */
import{Kind as r}from"./374bb6ac2eb6f198.mjs";function o(t,e){let i=null;for(let n of t.definitions)if(n.kind===r.OPERATION_DEFINITION){if(e==null){if(i)return null;i=n}else if(n.name?.value===e)return n}return i}export{o as getOperationAST};
