package imports

// Queries contains tree-sitter query patterns for TypeScript import and export extraction.
//
// These patterns match all import and export statements in TypeScript/JavaScript code,
// including ES6 modules, type-only imports, and various syntactic forms.
//
// Captures:
//   - @import.* - Import-related nodes
//   - @export.* - Export-related nodes
//   - @source - Module source strings
const TSQueries = `
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
; TYPE-ONLY IMPORTS (TypeScript-specific)
; ===========================================================================

; Type-only import statement: import type { Foo, Bar } from './types';
; Marks entire import as type-only
(import_statement
  "type" @import.type.marker
  source: (string (string_fragment) @import.type.source)
)

; Type-only named imports: import type { Foo } from './types';
(import_statement
  "type"
  (import_clause
    (named_imports
      (import_specifier
        name: (identifier) @import.type.named
      )
    )
  )
)

; Per-symbol type import: import { type Foo } from './types';
; Individual imports marked with 'type' keyword
(import_specifier
  "type" @import.type.specifier.marker
  name: (identifier) @import.type.specifier.name
)

; Per-symbol type import with alias: import { type Foo as F } from './types';
(import_specifier
  "type"
  name: (identifier) @import.type.specifier.named
  alias: (identifier) @import.type.specifier.alias
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
    name: (type_identifier) @export.name
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

; TypeScript interface export: export interface User {}
(export_statement
  declaration: (interface_declaration
    name: (type_identifier) @export.name
  ) @export.declaration
)

; TypeScript type alias export: export type ID = string;
(export_statement
  declaration: (type_alias_declaration
    name: (type_identifier) @export.name
  ) @export.declaration
)

; TypeScript enum export: export enum Color {}
(export_statement
  declaration: (enum_declaration
    name: (identifier) @export.name
  ) @export.declaration
)
`
