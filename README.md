# xpr-go — XPR Expression Language for Go

> 🚧 **Coming Soon** — This runtime is not yet implemented.

[![CI](https://github.com/xpr-lang/xpr-go/actions/workflows/ci.yml/badge.svg)](https://github.com/xpr-lang/xpr-go/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

XPR is a safe, sandboxed expression language for evaluating user-defined expressions against structured data. This is the Go runtime.

## Planned API

```go
import xpr "github.com/xpr-lang/xpr-go"

engine := xpr.New()

result, err := engine.Evaluate(`items.filter(x => x.price > 50).map(x => x.name)`, map[string]any{
    "items": []map[string]any{
        {"name": "Widget", "price": 99},
        {"name": "Gadget", "price": 25},
    },
})
```

## Status

| Feature | Status |
|---------|--------|
| Tokenizer | 🔜 Planned |
| Pratt Parser | 🔜 Planned |
| Tree-walk Evaluator | 🔜 Planned |
| Built-in Functions | 🔜 Planned |
| Conformance Tests | 🔜 Planned |

## Specification

See [xpr-lang/xpr](https://github.com/xpr-lang/xpr) for the full language specification and conformance test suite.

## Contributing

Contributions welcome! See the spec repo for language design decisions.

## License

MIT — see [LICENSE](LICENSE)
