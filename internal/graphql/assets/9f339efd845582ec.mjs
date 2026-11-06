/* esm.sh - graphql@17.1.0/jsutils/memoize2 */
function d(u){let t;return function(i,f){t??=new WeakMap;let e=t.get(i);e===void 0&&(e=new WeakMap,t.set(i,e));let n=e.get(f);return n===void 0&&(n=u(i,f),e.set(f,n)),n}}export{d as memoize2};
