/* esm.sh - graphql@17.1.0/execution/createSharedExecutionContext */
import{AsyncWorkTracker as c}from"./bdf9d9162966e01c.mjs";function a(n){let e=new c,o,t=r=>e.promiseAllTrackOnReject(r);return{asyncWorkTracker:e,getAbortSignal:()=>n,getAsyncHelpers:()=>o??={promiseAll:t,track:r=>e.addValues(r)},promiseAll:t}}export{a as createSharedExecutionContext};
