/* esm.sh - graphql@17.1.0/error/locatedError */
import{toError as e}from"./ef6a645deaab4112.mjs";import{GraphQLError as i}from"./3f07b35bd75fe31a.mjs";function c(o,t,n){let r=e(o);return s(r)?r:new i(r.message,{nodes:r.nodes??t,source:r.source,positions:r.positions,path:n,originalError:r})}function s(o){return Array.isArray(o.path)}export{c as locatedError};
