// Package catalogs provides embedded pre-built catalog data for supported UI libraries.
package catalogs

import _ "embed"

// ShadcnJSON is the bundled shadcn/ui catalog, embedded at build time.
//
//go:embed shadcn/catalog.json
var ShadcnJSON []byte
