/* esm.sh - graphql@17.1.0/execution/collectIteratorPromises */
import{isPromiseLike as o}from"./a199619a4472f30d.mjs";function n(r){let e=[];try{for(;;){let t=r.next();if(t.done)return e;o(t.value)&&e.push(t.value)}}catch{return e}}export{n as collectIteratorPromises};
