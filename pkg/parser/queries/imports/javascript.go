package imports

// Queries contains tree-sitter query patterns for JavaScript import and export extraction.
//
// Pure JavaScript (ES6 modules) - excludes TypeScript type system features.
// These patterns match all import and export statements in JavaScript code.
//
// Captures:
//   - @import.* - Import-related nodes
//   - @export.* - Export-related nodes
//   - @source - Module source strings
const JSQueries = `
; ===========================================================================
; IMPORT STATEMENTS
; ===========================================================================

; Import source - capture from all import types
(import_statement
  source: (string (string_fragment) @import.source)
)

; Named imports: import { foo, bar, baz as b } from './utils';
; Captures both name and alias (alias may or may not be present)
(import_specifier
  name: (identifier) @import.named
)

; Named import aliases: import { foo as f } from './utils';
; Separate capture for aliases
(import_specifier
  alias: (identifier) @import.alias
)

; Default import: import React from 'react';
(import_statement
  (import_clause
    (identifier) @import.default
  )
)

; Namespace import: import * as utils from './utils';
(import_statement
  (import_clause
    (namespace_import
      (identifier) @import.namespace
    )
  )
)

; ===========================================================================
; EXPORT STATEMENTS
; ===========================================================================

; Named function export: export function foo() {}
(export_statement
  declaration: (function_declaration
    name: (identifier) @export.name
  ) @export.declaration
)

; Named class export: export class MyClass {}
(export_statement
  declaration: (class_declaration
    name: (identifier) @export.name
  ) @export.declaration
)

; Named variable export: export const foo = 1;
(export_statement
  declaration: (lexical_declaration
    (variable_declarator
      name: (identifier) @export.name
    )
  ) @export.declaration
)

; Default export with function: export default function() {}
; Capture the function_expression as both declaration and give it a default name
(export_statement
  value: (function_expression) @export.declaration
) @export.default

; Default export with identifier: export default foo;
(export_statement
  value: (identifier) @export.default
)

; Export list names: export { foo, bar };
; Match individual specifiers without source
(export_specifier
  name: (identifier) @export.name
)

; Re-export source capture
(export_statement
  source: (string (string_fragment) @export.reexport.source)
)

; Re-export names: export { foo, bar } from './other';
; Match individual specifiers within re-export
(export_statement
  source: (string)
) @export.reexport.marker

(export_statement
  (export_clause
    (export_specifier
      name: (identifier) @export.reexport.name
    )
  )
  source: (string)
)

; Re-export all: export * from './other';
(export_statement
  !declaration
  source: (string (string_fragment) @export.reexport.source)
)

; ===========================================================================
; COMMONJS IMPORTS (require)
; ===========================================================================
; CommonJS uses standard JavaScript nodes (not special syntax):
; - require() is a regular call_expression
; - module.exports is a member_expression with assignment_expression
;
; These patterns enable support for popular JavaScript libraries that use
; CommonJS (lodash, express, etc.)

; Simple require: const foo = require('./module')
; Treat as namespace import (entire module bound to identifier)
(lexical_declaration
  (variable_declarator
    name: (identifier) @import.commonjs.namespace
    value: (call_expression
      function: (identifier) @_require (#eq? @_require "require")
      arguments: (arguments
        (string (string_fragment) @import.commonjs.source)
      )
    )
  )
)

; Destructured require - shorthand: const { bar } = require('./module')
; Each property is a separate named import
(lexical_declaration
  (variable_declarator
    name: (object_pattern
      (shorthand_property_identifier_pattern) @import.commonjs.named
    )
    value: (call_expression
      function: (identifier) @_require (#eq? @_require "require")
      arguments: (arguments
        (string (string_fragment) @import.commonjs.source)
      )
    )
  )
)

; Destructured require - with alias: const { bar: baz } = require('./module')
; bar is the exported name, baz is the local binding
(lexical_declaration
  (variable_declarator
    name: (object_pattern
      (pair_pattern
        key: (property_identifier) @import.commonjs.key
        value: (identifier) @import.commonjs.value
      )
    )
    value: (call_expression
      function: (identifier) @_require (#eq? @_require "require")
      arguments: (arguments
        (string (string_fragment) @import.commonjs.source)
      )
    )
  )
)

; Member access require: const bar = require('./module').bar
; Import specific property from module
(lexical_declaration
  (variable_declarator
    name: (identifier) @import.commonjs.named
    value: (member_expression
      object: (call_expression
        function: (identifier) @_require (#eq? @_require "require")
        arguments: (arguments
          (string (string_fragment) @import.commonjs.source)
        )
      )
      property: (property_identifier) @import.commonjs.property
    )
  )
)

; ===========================================================================
; COMMONJS EXPORTS
; ===========================================================================
; CommonJS exports use assignment to module.exports or exports object.
; We extract the exported names to build the export graph.

; module.exports = value (default export)
; Assigns entire module.exports to a single value
(assignment_expression
  left: (member_expression
    object: (identifier) @_module (#eq? @_module "module")
    property: (property_identifier) @_exports (#eq? @_exports "exports")
  )
  right: (identifier) @export.commonjs.default
)

; module.exports = { foo, bar } - shorthand properties
; Object literal with shorthand property names
(assignment_expression
  left: (member_expression
    object: (identifier) @_module (#eq? @_module "module")
    property: (property_identifier) @_exports (#eq? @_exports "exports")
  )
  right: (object
    (shorthand_property_identifier) @export.commonjs.name
  )
)

; module.exports = { foo: value } - full properties
; Object literal with explicit key-value pairs
(assignment_expression
  left: (member_expression
    object: (identifier) @_module (#eq? @_module "module")
    property: (property_identifier) @_exports (#eq? @_exports "exports")
  )
  right: (object
    (pair
      key: (property_identifier) @export.commonjs.name
    )
  )
)

; exports.foo = value
; Direct property assignment to exports object
(assignment_expression
  left: (member_expression
    object: (identifier) @_exports (#eq? @_exports "exports")
    property: (property_identifier) @export.commonjs.name
  )
)

; module.exports.foo = value
; Property assignment to module.exports
(assignment_expression
  left: (member_expression
    object: (member_expression
      object: (identifier) @_module (#eq? @_module "module")
      property: (property_identifier) @_exports (#eq? @_exports "exports")
    )
    property: (property_identifier) @export.commonjs.name
  )
)
`
