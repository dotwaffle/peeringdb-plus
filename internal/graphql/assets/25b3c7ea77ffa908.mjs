/* esm.sh - graphql@17.1.0/error/ensureGraphQLError */
import{toError as e}from"./ef6a645deaab4112.mjs";import{GraphQLError as n}from"./3f07b35bd75fe31a.mjs";function f(r){if(r instanceof n)return r;let o=e(r);return new n(o.message,{originalError:o})}export{f as ensureGraphQLError};
