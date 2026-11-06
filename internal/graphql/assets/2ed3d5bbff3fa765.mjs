/* esm.sh - graphql@17.1.0/utilities/printSchema */
import{inspect as b}from"./2b01d27f86c0cc25.mjs";import{invariant as j}from"./65d4738a32b1de64.mjs";import{isPrintableAsBlockString as y}from"./0910df89f31fe74b.mjs";import{Kind as s}from"./374bb6ac2eb6f198.mjs";import{print as u}from"./aed7a41d29b71e60.mjs";import{isEnumType as R,isInputObjectType as $,isInterfaceType as D,isObjectType as I,isScalarType as O,isUnionType as A}from"./593b35bd868e4b3b.mjs";import{DEFAULT_DEPRECATION_REASON as V,isSpecifiedDirective as l}from"./65f7f1c939782880.mjs";import{isIntrospectionType as m}from"./a8265231a83f9039.mjs";import{isSpecifiedScalarType as k}from"./a48888c63f3bcf02.mjs";import{getDefaultValueAST as v}from"./3ad7f474d6406521.mjs";function Z(n){return T(n,e=>!l(e),x)}function h(n){return T(n,l,m)}function x(n){return!k(n)&&!m(n)}function T(n,e,t){let r=n.getDirectives().filter(e),c=Object.values(n.getTypeMap()).filter(t);return[B(n),...r.map(o=>F(o)),...c.map(o=>E(o))].filter(Boolean).join(`

`)}function B(n){let e=n.getQueryType(),t=n.getMutationType(),r=n.getSubscriptionType();if(!(!e&&!t&&!r)&&(n.description!=null||!U(n)))return i(n)+`schema {
`+(e?`  query: ${e}
`:"")+(t?`  mutation: ${t}
`:"")+(r?`  subscription: ${r}
`:"")+"}"}function U(n){return n.getQueryType()==n.getType("Query")&&n.getMutationType()==n.getType("Mutation")&&n.getSubscriptionType()==n.getType("Subscription")}function E(n){if(O(n))return N(n);if(I(n))return L(n);if(D(n))return M(n);if(A(n))return G(n);if(R(n))return Q(n);if($(n))return q(n);j(!1,"Unexpected type: "+b(n))}function N(n){return i(n)+`scalar ${n}`+P(n)}function S(n){let e=n.getInterfaces();return e.length?" implements "+e.map(t=>t.name).join(" & "):""}function L(n){return i(n)+`type ${n}`+S(n)+g(n)}function M(n){return i(n)+`interface ${n}`+S(n)+g(n)}function G(n){let e=n.getTypes(),t=e.length?" = "+e.join(" | "):"";return i(n)+`union ${n.name}`+t}function Q(n){let e=n.getValues().map((t,r)=>i(t,"  ",!r)+"  "+t.name+p(t.deprecationReason));return i(n)+`enum ${n}`+a(e)}function q(n){let e=Object.values(n.getFields()).map((t,r)=>i(t,"  ",!r)+"  "+f(t));return i(n)+`input ${n}`+(n.isOneOf?" @oneOf":"")+a(e)}function g(n){let e=Object.values(n.getFields()).map((t,r)=>i(t,"  ",!r)+"  "+t.name+d(t.args,"  ")+": "+String(t.type)+p(t.deprecationReason));return a(e)}function a(n){return n.length!==0?` {
`+n.join(`
`)+`
}`:""}function d(n,e=""){return n.length===0?"":n.every(t=>t.description==null)?"("+n.map(f).join(", ")+")":`(
`+n.map((t,r)=>i(t,"  "+e,!r)+"  "+e+f(t)).join(`
`)+`
`+e+")"}function f(n){let e=n.name+": "+String(n.type),t=v(n);return t&&(e+=` = ${u(t)}`),e+p(n.deprecationReason)}function F(n){return i(n)+`directive ${n}`+d(n.args)+p(n.deprecationReason)+(n.isRepeatable?" repeatable":"")+" on "+n.locations.join(" | ")}function p(n){return n==null?"":n!==V?` @deprecated(reason: ${u({kind:s.STRING,value:n})})`:" @deprecated"}function P(n){return n.specifiedByURL==null?"":` @specifiedBy(url: ${u({kind:s.STRING,value:n.specifiedByURL})})`}function i(n,e="",t=!0){let{description:r}=n;if(r==null)return"";let c=u({kind:s.STRING,value:r,block:y(r)});return(e&&!t?`
`+e:e)+c.replaceAll(`
`,`
`+e)+`
`}export{F as printDirective,h as printIntrospectionSchema,Z as printSchema,E as printType};
