/* esm.sh - graphql@17.1.0/jsutils/promiseReduce */
import{isPromise as m}from"./a199619a4472f30d.mjs";function n(t,e,u){let o=u;for(let r of t)o=m(o)?o.then(i=>e(i,r)):e(o,r);return o}export{n as promiseReduce};
