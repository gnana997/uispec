package symbols

// Queries contains tree-sitter query patterns for JavaScript symbol extraction.
//
// These patterns match JavaScript AST nodes to extract symbols (functions, classes,
// methods, variables) with their names and locations.
//
// Note: JavaScript grammar is very similar to TypeScript but without type annotations.
//
// Each query captures:
//   - @name - The symbol name
//   - @definition - The entire symbol node (for location)
const JSQueries = `
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

; Generator functions
; function* myGenerator() { ... }
(generator_function_declaration
  name: (identifier) @function.name
) @function.definition

; ============================================================================
; Classes
; ============================================================================

; Class declarations
; class MyClass { ... }
(class_declaration
  name: (identifier) @class.name
  body: (class_body) @body
) @class.definition

; Class expressions
; const MyClass = class { ... }
(variable_declarator
  name: (identifier) @class.name
  value: (class)
) @class.definition

; ============================================================================
; Methods
; ============================================================================

; Method definitions in classes
; class MyClass { myMethod() { ... } }
(method_definition
  name: (property_identifier) @method.name
) @method.definition

; Getter methods
; class MyClass { get myProp() { ... } }
(method_definition
  name: (property_identifier) @method.name
  parameters: (formal_parameters)
) @method.definition

; Setter methods
; class MyClass { set myProp(value) { ... } }
(method_definition
  name: (property_identifier) @method.name
  parameters: (formal_parameters)
) @method.definition

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

; Old-style var declarations
; var myOldVar = 42;
(variable_declaration
  (variable_declarator
    name: (identifier) @variable.name
  ) @variable.definition
)

; ============================================================================
; Object Properties (for module exports)
; ============================================================================

; Object property methods
; const obj = { myMethod() { ... } }
(pair
  key: (property_identifier) @function.name
  value: (function_expression)
) @function.definition

; Object property arrow functions
; const obj = { myMethod: () => { ... } }
(pair
  key: (property_identifier) @function.name
  value: (arrow_function)
) @function.definition
`
