/* esm.sh - graphql@17.1.0/jsutils/toError */
import{inspect as o}from"./2b01d27f86c0cc25.mjs";function s(r){return r instanceof Error?r:new e(r)}var e=class extends Error{constructor(t){super("Unexpected error value: "+o(t)),this.name="NonErrorThrown",this.thrownValue=t}};export{s as toError};
