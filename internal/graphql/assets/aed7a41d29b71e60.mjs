/* esm.sh - graphql@17.1.0/language/printer */
import{printBlockString as w}from"./0910df89f31fe74b.mjs";import{printString as T}from"./f587c444f5f8a417.mjs";import{reduce as y}from"./bcbfb6e331582c83.mjs";function E(n,t){return S(y(n,I),t?.maxIndentDepth??1/0)}function F(n){return E(n,{maxIndentDepth:100})}var d=80,s=class n{constructor(t="",i=!1){this.next=void 0,this.tail=this,this.indentBefore=!1,this.dedentAfter=!1,this.length=t.length,this.lines=t.includes(`
`)?t.split(`
`):t,this.newlines=typeof this.lines=="string"?0:this.lines.length-1,this.isBlockString=i}append(t){if(t.length===0)return this;let i=typeof t=="string"?new n(t):t;return this.tail.next=i,this.tail=i.tail,this.length+=i.length,this.newlines+=i.newlines,this}indent(){return this.length===0?this:(this.indentBefore=!0,this.tail.dedentAfter=!0,this.length+=2*(this.newlines+1),this)}},m=new s;function S(n,t){let i="",o="",l=0,p="",a=n;for(;a!==void 0;){let f=a.lines;if(a.indentBefore&&(l<t&&(p+="  ",o+="  "),l++),typeof f=="string")o+=f;else{o+=f[0];let x=!a.isBlockString&&l>=t?" ":`
`+p;for(let c=1;c<f.length;c++)i+=o,o=x+f[c]}a.dedentAfter&&(l--,p=p.slice(0,l*2)),a=a.next}return i+o}var I={Name:n=>new s(n.value),Variable:n=>e(["$",n.name]),Document:n=>e(n.definitions,`

`),OperationDefinition(n){let t=h(n.variableDefinitions)?r(`(
`,e(n.variableDefinitions,`
`),`
)`):r("(",e(n.variableDefinitions,", "),")"),i=e([r("",n.description,`
`),e([n.operation,e([n.name,t]),e(n.directives," ")]," ")]);return e([n.operation==="query"&&i.length===n.operation.length?"":e([i," "]),n.selectionSet])},VariableDefinition:({variable:n,type:t,defaultValue:i,directives:o,description:l})=>e([r("",l,`
`),n,": ",t,r(" = ",i),r(" ",e(o," "))]),SelectionSet:({selections:n})=>n.length===0?new s("{}"):u(n),Field({alias:n,name:t,arguments:i,directives:o,selectionSet:l}){let p=e([r("",n,": "),t],"");return e([D(p,i),r(" ",e(o," ")),r(" ",l)])},Argument:({name:n,value:t})=>e([n,": ",t]),FragmentArgument:({name:n,value:t})=>e([n,": ",t]),FragmentSpread:({name:n,arguments:t,directives:i})=>{let o=e(["...",n]);return e([D(o,t),r(" ",e(i," "))])},InlineFragment:({typeCondition:n,directives:t,selectionSet:i})=>e(["...",r("on ",n),e(t," "),i]," "),FragmentDefinition:({name:n,typeCondition:t,variableDefinitions:i,directives:o,selectionSet:l,description:p})=>e([r("",p,`
`),"fragment ",n,r("(",e(i,", "),")")," on ",t," ",r("",e(o," ")," "),l]),IntValue:({value:n})=>new s(n),FloatValue:({value:n})=>new s(n),StringValue:({value:n,block:t})=>new s(t===!0?w(n):T(n),t===!0),BooleanValue:({value:n})=>new s(n?"true":"false"),NullValue:()=>new s("null"),EnumValue:({value:n})=>new s(n),ListValue:({values:n})=>2+g(n,", ")>d?r(`[
`,e(n,`
`).indent(),`
]`):e(["[",e(n,", "),"]"]),ObjectValue:({fields:n})=>4+g(n,", ")>d?u(n):e(["{ ",e(n,", ")," }"]),ObjectField:({name:n,value:t})=>e([n,": ",t]),Directive:({name:n,arguments:t})=>e(["@",n,r("(",e(t,", "),")")]),NamedType:({name:n})=>n,ListType:({type:n})=>e(["[",n,"]"]),NonNullType:({type:n})=>e([n,"!"]),SchemaDefinition:({description:n,directives:t,operationTypes:i})=>e([r("",n,`
`),e(["schema",e(t," "),u(i)]," ")]),OperationTypeDefinition:({operation:n,type:t})=>e([n,": ",t]),ScalarTypeDefinition:({description:n,name:t,directives:i})=>e([r("",n,`
`),e(["scalar",t,e(i," ")]," ")]),ObjectTypeDefinition:({description:n,name:t,interfaces:i,directives:o,fields:l})=>e([r("",n,`
`),e(["type",t,r("implements ",e(i," & ")),e(o," "),u(l)]," ")]),FieldDefinition:({description:n,name:t,arguments:i,type:o,directives:l})=>e([r("",n,`
`),t,h(i)?r(`(
`,e(i,`
`).indent(),`
)`):r("(",e(i,", "),")"),": ",o,r(" ",e(l," "))]),InputValueDefinition:({description:n,name:t,type:i,defaultValue:o,directives:l})=>e([r("",n,`
`),t,": ",i,r(" = ",o),r(" ",e(l," "))]),InterfaceTypeDefinition:({description:n,name:t,interfaces:i,directives:o,fields:l})=>e([r("",n,`
`),e(["interface",t,r("implements ",e(i," & ")),e(o," "),u(l)]," ")]),UnionTypeDefinition:({description:n,name:t,directives:i,types:o})=>e([r("",n,`
`),e(["union",t,e(i," "),r("= ",e(o," | "))]," ")]),EnumTypeDefinition:({description:n,name:t,directives:i,values:o})=>e([r("",n,`
`),e(["enum",t,e(i," "),u(o)]," ")]),EnumValueDefinition:({description:n,name:t,directives:i})=>e([r("",n,`
`),e([t,e(i," ")]," ")]),InputObjectTypeDefinition:({description:n,name:t,directives:i,fields:o})=>e([r("",n,`
`),e(["input",t,e(i," "),u(o)]," ")]),DirectiveDefinition:({description:n,name:t,arguments:i,directives:o,repeatable:l,locations:p})=>e([r("",n,`
`),"directive @",t,h(i)?r(`(
`,e(i,`
`).indent(),`
)`):r("(",e(i,", "),")"),r(" ",e(o," ")),l?" repeatable":""," on ",e(p," | ")]),SchemaExtension:({directives:n,operationTypes:t})=>e(["extend schema",e(n," "),u(t)]," "),ScalarTypeExtension:({name:n,directives:t})=>e(["extend scalar",n,e(t," ")]," "),ObjectTypeExtension:({name:n,interfaces:t,directives:i,fields:o})=>e(["extend type",n,r("implements ",e(t," & ")),e(i," "),u(o)]," "),InterfaceTypeExtension:({name:n,interfaces:t,directives:i,fields:o})=>e(["extend interface",n,r("implements ",e(t," & ")),e(i," "),u(o)]," "),UnionTypeExtension:({name:n,directives:t,types:i})=>e(["extend union",n,e(t," "),r("= ",e(i," | "))]," "),EnumTypeExtension:({name:n,directives:t,values:i})=>e(["extend enum",n,e(t," "),u(i)]," "),InputObjectTypeExtension:({name:n,directives:t,fields:i})=>e(["extend input",n,e(t," "),u(i)]," "),DirectiveExtension:({name:n,directives:t})=>e([e(["extend directive @",n]),e(t," ")]," "),TypeCoordinate:({name:n})=>n,MemberCoordinate:({name:n,memberName:t})=>e([n,r(".",t)]),ArgumentCoordinate:({name:n,fieldName:t,argumentName:i})=>e([n,r(".",t),r("(",i,":)")]),DirectiveCoordinate:({name:n})=>e(["@",n]),DirectiveArgumentCoordinate:({name:n,argumentName:t})=>e(["@",n,r("(",t,":)")])};function e(n,t=""){if(n==null||n.length===0)return m;let i=m;for(let o of n)o===void 0||o.length===0||(i.length===0?i=typeof o=="string"?new s(o):o:(t!==""&&i.append(t),i.append(o)));return i}function g(n,t){let i=0;for(let o of n)o.length!==0&&(i+=(i===0?0:t.length)+o.length);return i}function u(n){return r(`{
`,e(n,`
`).indent(),`
}`)}function r(n,t,i=""){return t!=null&&t.length!==0?n===""?t.append(i):new s(n).append(t).append(i):m}function h(n){return n?.some(t=>t.newlines!==0)??!1}function D(n,t){return t==null||t.length===0?n:e([n,n.length+g(t,", ")+2>d?r(`(
`,e(t,`
`).indent(),`
)`):r("(",e(t,", "),")")])}export{E as print,F as printForError};
