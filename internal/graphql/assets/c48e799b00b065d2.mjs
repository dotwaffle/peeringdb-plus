/* esm.sh - graphql@17.1.0/validation/rules/KnownOperationTypesRule */
import{GraphQLError as n}from"./3f07b35bd75fe31a.mjs";function i(o){let t=o.getSchema();return{OperationDefinition(e){let r=e.operation;t.getRootType(r)||o.reportError(new n(`The ${r} operation is not supported by the schema.`,{nodes:e}))}}}export{i as KnownOperationTypesRule};
