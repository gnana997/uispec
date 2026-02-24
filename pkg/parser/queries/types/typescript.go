package types

// TSQueries contains tree-sitter query patterns for TypeScript type annotation extraction.
//
// These patterns match TypeScript AST nodes to extract explicit type annotations from
// variable declarations, function parameters, class properties, and other constructs.
//
// This enables method call resolution by looking up the declared type of receiver variables.
//
// Example resolution flow:
//
//	Source:  const service: UserService = new UserService();
//	         service.getUser(id);
//
//	Extract: service → UserService
//	Resolve: service.getUser() → UserService.getUser()
//
// Supported annotation patterns:
//   - Variable declarations:  const x: Type = ...
//   - Function parameters:    function f(x: Type) { ... }
//   - Class properties:       private x: Type;
//   - Generic types:          Array<User> → extracts "User" (first type argument)
//   - Nested types:           models.User
//   - Predefined types:       string, number, boolean, etc.
//
// Strategy for complex types:
//   - Generics: Extract first type argument (Array<User> → User)
//   - Unions/Intersections: Skip initially
//   - Conditional types: Skip initially
//
// Each query captures:
//   - @type.var.name - The variable/parameter/property name
//   - @type.name - The type name (simple types)
//   - @type.base - The base type for generics (e.g., "Array" in Array<User>)
//   - @type.arg - The first type argument for generics (e.g., "User" in Array<User>)
const TSQueries = `
; ============================================================================
; Variable Declarations with Type Annotations
; ============================================================================

; Simple variable with type annotation
; const service: UserService = ...
(lexical_declaration
  (variable_declarator
    name: (identifier) @type.var.name
    type: (type_annotation
      (type_identifier) @type.name)))

; Variable with predefined type
; const count: number = 0
(lexical_declaration
  (variable_declarator
    name: (identifier) @type.var.name
    type: (type_annotation
      (predefined_type) @type.name)))

; Variable with nested/qualified type
; const user: models.User = ...
(lexical_declaration
  (variable_declarator
    name: (identifier) @type.var.name
    type: (type_annotation
      (nested_type_identifier) @type.name)))

; ============================================================================
; Generic Types - Extract ALL Type Arguments
; ============================================================================

; Generic type - extract all type arguments using + quantifier
; const users: Array<User> = ... → extract "User"
; const map: Map<string, number> = ... → extract "string", "number"
(lexical_declaration
  (variable_declarator
    name: (identifier) @type.var.name
    type: (type_annotation
      (generic_type
        name: (_) @type.base
        type_arguments: (type_arguments
          (type_identifier)+ @type.arg)))))

; Generic type with predefined types (string, number, etc.)
; const map: Map<string, number> = ... → extract "string", "number"
(lexical_declaration
  (variable_declarator
    name: (identifier) @type.var.name
    type: (type_annotation
      (generic_type
        name: (_) @type.base
        type_arguments: (type_arguments
          (predefined_type)+ @type.arg)))))

; ============================================================================
; Function Parameters with Type Annotations
; ============================================================================

; Required parameter with type
; function process(data: DataType) { ... }
(required_parameter
  pattern: (identifier) @type.var.name
  type: (type_annotation
    (type_identifier) @type.name))

; Required parameter with predefined type
; function log(message: string) { ... }
(required_parameter
  pattern: (identifier) @type.var.name
  type: (type_annotation
    (predefined_type) @type.name))

; Required parameter with generic type - extract all type arguments
; function process(items: Array<Item>) { ... }
(required_parameter
  pattern: (identifier) @type.var.name
  type: (type_annotation
    (generic_type
      type_arguments: (type_arguments
        (type_identifier)+ @type.arg))))

; Optional parameter with type
; function format(value?: number) { ... }
(optional_parameter
  pattern: (identifier) @type.var.name
  type: (type_annotation
    (type_identifier) @type.name))

; ============================================================================
; Class Properties with Type Annotations
; ============================================================================

; Public/private field with type
; class MyClass {
;   private service: UserService;
; }
(public_field_definition
  name: (property_identifier) @type.var.name
  type: (type_annotation
    (type_identifier) @type.name))

; Field with predefined type
; class MyClass {
;   public count: number = 0;
; }
(public_field_definition
  name: (property_identifier) @type.var.name
  type: (type_annotation
    (predefined_type) @type.name))

; Field with generic type - extract all type arguments
; class MyClass {
;   private users: Array<User> = [];
; }
(public_field_definition
  name: (property_identifier) @type.var.name
  type: (type_annotation
    (generic_type
      type_arguments: (type_arguments
        (type_identifier)+ @type.arg))))

; ============================================================================
; Arrow Functions - Parameters in Formal Parameters
; ============================================================================

; Arrow function parameters are handled by required_parameter patterns above
; const handler = (req: Request, res: Response) => { ... }
; These will be matched by the required_parameter patterns

; ============================================================================
; Type Assertions (as keyword)
; ============================================================================

; Type assertion - extract target type
; const service = obj as UserService
(lexical_declaration
  (variable_declarator
    name: (identifier) @type.var.name
    value: (as_expression
      (type_identifier) @type.name)))

; Type assertion with predefined type
; const count = value as number
(lexical_declaration
  (variable_declarator
    name: (identifier) @type.var.name
    value: (as_expression
      (predefined_type) @type.name)))

; ============================================================================
; Intersection Types (A & B)
; ============================================================================

; Function parameter with intersection type - Generic version
; function f(opts: Partial<Observer> & RequestOptions) { ... }
; Strategy: Match the intersection_type parent, then extract ALL identifiers inside type_arguments
; This captures "Observer" from Partial<Observer> in the intersection
(required_parameter
  pattern: (identifier) @type.var.name
  type: (type_annotation
    (intersection_type
      (generic_type
        type_arguments: (type_arguments
          (_) @type.arg)))))

; ============================================================================
; Object Binding Patterns (Destructuring)
; ============================================================================

; Destructured parameter with type
; function process({ user }: { user: User }) { ... }
; Note: This is complex - defer to Phase 4 initially

; ============================================================================
; Notes on Capture Indices
; ============================================================================
; Captures are processed in order:
;   @type.var.name (index 0) - Variable/parameter/property name
;   @type.name     (index 1) - Type name (simple types)
;   @type.base     (index 2) - Base type for generics (Array, Map, etc.)
;   @type.arg      (index 3) - First type argument (User in Array<User>)
;
; Priority: type.arg > type.name > type.base
;   - If type.arg exists, use it (User from Array<User>)
;   - Else if type.name exists, use it (UserService)
;   - Else if type.base exists, use it (Array from Array<User>)
`
