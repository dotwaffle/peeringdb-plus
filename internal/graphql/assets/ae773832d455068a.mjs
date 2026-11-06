/* esm.sh - graphql@17.1.0/jsutils/promiseForObject */
function l(t,c){let n=Object.keys(t),s=Object.values(t);return c(s).then(o=>{let r=Object.create(null);for(let e=0;e<n.length;++e)r[n[e]]=o[e];return r})}export{l as promiseForObject};
