# xpr-go — XPR Expression Language for Go

[![CI](https://github.com/xpr-lang/xpr-go/actions/workflows/ci.yml/badge.svg)](https://github.com/xpr-lang/xpr-go/actions/workflows/ci.yml)
[![XPR spec](https://img.shields.io/badge/XPR_spec-v0.2-blue)](https://github.com/xpr-lang/xpr)
[![conformance](https://img.shields.io/badge/conformance-100%25-brightgreen)](https://github.com/xpr-lang/xpr/tree/main/conformance)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**XPR** is a sandboxed cross-language expression language for data pipeline transforms. This is the Go reference runtime.

## Install

```bash
go get github.com/xpr-lang/xpr-go
```

## Quick Start

```go
package main

import (
	"fmt"
	"github.com/xpr-lang/xpr-go"
)

func main() {
	engine := xpr.New()

	result, err := engine.Evaluate(`items.filter(x => x.price > 50).map(x => x.name)`, map[string]any{
		"items": []map[string]any{
			{"name": "Widget", "price": 25},
			{"name": "Gadget", "price": 75},
			{"name": "Doohickey", "price": 100},
		},
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(result) // → ["Gadget", "Doohickey"]
}
```

## API

### `Evaluate(expression string, context map[string]any) (any, error)`

Evaluates an XPR expression against an optional context object.

```go
engine := xpr.New()

result, _ := engine.Evaluate("1 + 2", nil)                          // → 3.0
result, _ := engine.Evaluate("user.name", map[string]any{"user": map[string]any{"name": "Alice"}}) // → "Alice"
result, _ := engine.Evaluate("items.length", map[string]any{"items": []any{1, 2, 3}})     // → 3.0
```

Returns the result as `any`. Returns an error on parse or evaluation errors.

### `AddFunction(name string, fn func(...any) (any, error))`

Register a custom function callable from expressions:

```go
engine := xpr.New()

engine.AddFunction("double", func(args ...any) (any, error) {
	n := args[0].(float64)
	return n * 2, nil
})
engine.AddFunction("greet", func(args ...any) (any, error) {
	return fmt.Sprintf("Hello, %s!", args[0]), nil
})

result, _ := engine.Evaluate("double(21)", nil)           // → 42.0
result, _ := engine.Evaluate("greet(\"World\")", nil)     // → "Hello, World!"
result, _ := engine.Evaluate("items.map(x => double(x))", map[string]any{"items": []any{1, 2, 3}}) // → [2.0, 4.0, 6.0]
```

## Built-in Functions

**Math**: `round`, `floor`, `ceil`, `abs`, `min`, `max`

**Type**: `type`, `int`, `float`, `string`, `bool`

**String methods**: `.len()`, `.upper()`, `.lower()`, `.trim()`, `.startsWith()`, `.endsWith()`, `.contains()`, `.split()`, `.replace()`, `.slice()`, `.indexOf()`, `.repeat()`, `.trimStart()`, `.trimEnd()`, `.charAt()`, `.padStart()`, `.padEnd()`

**Array methods**: `.map()`, `.filter()`, `.reduce()`, `.find()`, `.some()`, `.every()`, `.flatMap()`, `.sort()`, `.reverse()`, `.length`, `.includes()`, `.indexOf()`, `.slice()`, `.join()`, `.concat()`, `.flat()`, `.unique()`, `.zip()`, `.chunk()`, `.groupBy()`

**Object methods**: `.keys()`, `.values()`, `.entries()`, `.has()`

**Utility**: `range()`

## v0.2 Features

**Let Bindings**: Immutable scoped bindings allow you to define and reuse values within expressions:

```go
result, _ := engine.Evaluate("let x = 1; let y = x + 1; y", nil) // → 2.0
result, _ := engine.Evaluate("let items = [1, 2, 3]; items.map(x => x * 2)", nil) // → [2.0, 4.0, 6.0]
```

**Spread Operator**: Spread syntax for arrays and objects enables composition and merging:

```go
result, _ := engine.Evaluate("[1, 2, ...[3, 4]]", nil) // → [1.0, 2.0, 3.0, 4.0]
result, _ := engine.Evaluate("{...{a: 1}, b: 2}", nil) // → {a: 1.0, b: 2.0}
```

## Conformance

This runtime supports **Level 1–3** (all conformance levels):
- Level 1: Literals, arithmetic, comparison, logic, ternary, property access, function calls
- Level 2: Arrow functions, collection methods, string methods, template literals
- Level 3: Pipe operator (`|>`), optional chaining (`?.`), nullish coalescing (`??`)

**v0.2 additions**: Let bindings, spread operator, 20 new built-in methods (10 array, 7 string, 2 object, 1 global)

## Specification

See the [XPR Language Specification](https://github.com/xpr-lang/xpr) for the full EBNF grammar, type system, operator precedence, and conformance test suite.

## License

MIT
