/* esm.sh - graphql@17.1.0/jsutils/didYouMean */
import{orList as a}from"./0a4e842560af0364.mjs";var g=5;function m(e,n){let[s,t]=n?[e,n]:[void 0,e];if(t.length===0)return"";let i=" Did you mean ";s!=null&&(i+=s+" ");let o=a(t.slice(0,g).map(u=>`"${u}"`));return i+o+"?"}export{m as didYouMean};
