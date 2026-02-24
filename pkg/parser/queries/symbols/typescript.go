package symbols

// Queries contains tree-sitter query patterns for TypeScript symbol extraction.
//
// These patterns match TypeScript AST nodes to extract symbols (functions, classes,
// methods, variables, types, interfaces, enums) with their names and locations.
//
// Each query captures:
//   - @name - The symbol name
//   - @definition - The entire symbol node (for location)
const TSQueries = `
; ============================================================================
; Functions
; ============================================================================

; Function declarations
; function myFunction() { ... }
(function_declaration
  name: (identifier) @function.name
) @function.definition

; Function expressions (captured as functions, not variables)
; const myFunc = function() { ... }
(variable_declarator
  name: (identifier) @function.name
  value: (function_expression)
) @function.definition

; Arrow functions (captured as variables, not functions)
; const myArrow = () => { ... }
; Note: Arrow functions are treated as variable declarations per test expectations
(variable_declarator
  name: (identifier) @variable.name
  value: (arrow_function)
) @variable.definition

; ============================================================================
; Classes
; ============================================================================

; Class declarations
; class MyClass { ... }
(class_declaration
  name: (type_identifier) @class.name
  body: (class_body) @body
) @class.definition

; Nested class expressions (static properties)
; class Application { static Logger = class { ... } }
(public_field_definition
  name: (property_identifier) @class.name
  value: (class)
) @class.definition

; ============================================================================
; Methods
; ============================================================================

; Method definitions in classes
; class MyClass { myMethod() { ... } }
(class_declaration
  body: (class_body
    (method_definition
      name: (property_identifier) @method.name
    ) @method.definition
  )
)

; Methods inside nested class expressions
; class Application { static Logger = class { info() {...} } }
(class
  body: (class_body
    (method_definition
      name: (property_identifier) @method.name
    ) @method.definition
  )
)

; ============================================================================
; Variables & Constants
; ============================================================================

; Variable declarations (let, const, var)
; const myVar = 42;
(lexical_declaration
  (variable_declarator
    name: (identifier) @variable.name
  ) @variable.definition
)

; ============================================================================
; Types & Interfaces
; ============================================================================

; Type aliases
; type MyType = string | number;
(type_alias_declaration
  name: (type_identifier) @type.name
) @type.definition

; Interface declarations
; interface MyInterface { ... }
(interface_declaration
  name: (type_identifier) @interface.name
) @interface.definition

; ============================================================================
; Enums
; ============================================================================

; Enum declarations
; enum MyEnum { A, B, C }
(enum_declaration
  name: (identifier) @enum.name
) @enum.definition
`
