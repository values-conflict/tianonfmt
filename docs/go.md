# Go style

Covers Go source files (`.go`) in the corpus.  `gofmt` handles all mechanical formatting — tabs, brace placement, line length, import grouping, etc. — so this document focuses only on conventions that are **beyond what `gofmt` enforces**.

## Import grouping

Imports are grouped into two blocks separated by a blank line:

1. Standard library packages
2. Third-party / internal packages

```go
import (
	"context"
	"errors"
	"fmt"

	"cuelabs.dev/go/oci/ociregistry"
	"cuelabs.dev/go/oci/ociregistry/ocimem"
)
```

`gofmt` (and `goimports`) enforce this two-group style automatically.  A third group for internal packages sometimes appears when internal packages are distinct from third-party ones, but this is not consistently observed in the corpus.

Corpus ref: [`meta-scripts/registry/lookup.go#L3-L10`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/lookup.go#L3-L10).

## Comments

**Exported symbols have doc comments** in the standard Go style (sentence starting with the symbol name).  The doc comment is dense and complete — it references related types using the `[TypeName]` link syntax:

```go
// a wrapper around [ociregistry.Interface.GetManifest] (and `GetTag`, `GetBlob`, and the `Resolve*` versions of the above) that accepts a [Reference] and always returns a [ociregistry.BlobReader] (in the case of a HEAD request, it will be a zero-length reader with just a valid descriptor)
func Lookup(ctx context.Context, ref Reference, opts *LookupOptions) (ociregistry.BlobReader, error) {
```

Corpus ref: [`meta-scripts/registry/lookup.go#L31-L32`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/lookup.go#L31-L32).

**Unexported functions and types** may or may not have doc comments.  When they do, the same style applies.

**Inline `// TODO` comments** are concrete and specific:

```go
// TODO allow providing a Descriptor here for more validation...
// TODO (also, if the provided Reference includes a Digest...)
```

Corpus ref: [`meta-scripts/registry/lookup.go#L27-L28`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/lookup.go#L27-L28).

## Struct field alignment

When a struct has fields of varying types that are logically related, the field names and tags are aligned:

```go
type ManifestChildren struct {
	Manifests []ocispec.Descriptor `json:"manifests"`

	Config *ocispec.Descriptor  `json:"config"` // pointer so we can recognize when it's not set
	Layers []ocispec.Descriptor `json:"layers"`
}
```

Note that `Config` has an extra space before `*ocispec.Descriptor` to align the type with `Layers`.  `gofmt` does not enforce this alignment for struct fields (only for `const` and `var` blocks), so this is a manual style choice.

Corpus ref: [`meta-scripts/registry/manifest-children.go#L9-L18`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/manifest-children.go#L9-L18).

## `const` blocks

Related constants are grouped in a single `const` block:

```go
const (
	LookupTypeManifest LookupType = "manifest"
	LookupTypeBlob     LookupType = "blob"
)
```

`gofmt` aligns the values in a const block automatically.

## `type` declarations

Named type aliases for semantic clarity:

```go
type LookupType string
```

This is preferred over using raw `string` everywhere when the string represents a constrained enumeration.

## Pointer semantics for optional struct fields

When a field can be "not set" (as distinct from a zero value), a pointer type is used:

```go
Config *ocispec.Descriptor `json:"config"` // have to turn this into a pointer so we can recognize when it's not set easier
```

This is documented explicitly in the comment.  The zero value of a non-pointer type would be ambiguous.

Corpus ref: [`meta-scripts/registry/manifest-children.go#L16`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/manifest-children.go#L16).

## Error handling

Standard Go error handling: explicit `if err != nil` checks, errors wrapped with `fmt.Errorf("context: %w", err)`:

```go
client, err := Client(ref.Host, nil)
if err != nil {
    return nil, fmt.Errorf("%s: failed getting client: %w", ref, err)
}
```

Error messages are lowercase (Go convention: error strings should not be capitalised or end with punctuation).

The format of error strings: `"context: description"` — the context identifies the operation, then a colon, then what failed.

Corpus ref: [`meta-scripts/registry/lookup.go#L33-L36`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/lookup.go#L33-L36).

## Variable declaration style

- Short declaration `:=` is used inside functions when the type is obvious from the right side
- `var` blocks are used when the zero value is meaningful or when declaring multiple related variables:

```go
var (
    r    ociregistry.BlobReader
    desc ociregistry.Descriptor
)
```

Corpus ref: [`meta-scripts/registry/lookup.go#L43-L46`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/registry/lookup.go#L43-L46).

## Package-level documentation

Complex packages have a doc comment at the top of a relevant file explaining the package's purpose.  Simple utility packages (`true.go`, `main.go` in trivial commands) have minimal or no package doc.

## Notable omissions

- `gofmt` is assumed — no non-gofmt formatting ever appears
- Named return values — not used; return values are unnamed
- `panic` for error handling — `panic` is used only for programmer errors (nil pointer dereference guards, etc.), never for runtime errors
- `interface{}` / `any` used sparingly — typed interfaces are preferred
- Global mutable state — avoided; state is passed explicitly or contained in structs
- `init()` functions — not observed in corpus
- Build tags / build constraints — not observed in corpus (though they would follow standard Go conventions)
