/* esm.sh - graphql@17.1.0/jsutils/memoize1 */
function f(i){let n;return function(t){n??=new WeakMap;let e=n.get(t);return e===void 0&&(e=i(t),n.set(t,e)),e}}export{f as memoize1};
