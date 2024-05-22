# Go Escape Lint

Go Escape Lint is a linter tool for Golang that checks for unsatisfied escape analysis constraints. 
While you can't force the Go compiler to inline a function or avoid heap allocations for a variable, you can at least verify if the compiler is optimizing your code as expected.

This tool helps quickly detect situations where performance-critical paths in the code are not optimized by the compiler due to unintended changes in the codebase, dependencies, or the compiler itself. 
This is achieved by introducing special code annotations and comparing those annotations to the actual escape analysis output of the compiler.

## Status

UNSTABLE. The rules are subject to change, and the tool is not yet widely tested.

## Supported Annotations

 * `//must-inline`: Checks if the function call is inlined at the call site.
 * `//no-escape`: Ensures that the declared variable does not escape to the heap.
 * `//no-bounds-check`: Ensures that the compiler does not insert bounds checks for the array or slice access.

## Usage

First, you need to install the linter tool:

```
go install github.com/maxpoletaev/go-escape-lint@latest
```

To extract useful information from the compiler, you need to provide some additional flags to the go build command.
The following flags enable escape analysis output, inlining information, and hints about bounds checks:

```
go build -gcflags="-m -d=ssa/check_bce" -o myapp 2>&1 | tee compiler-output.txt
```

Then you can run the linter tool on the output:

```
goescapelint -f compiler-output.txt
```

The result will show a list of places violating the annotations, if any:

```
go-escape-lint: variable at main.go:17 is marked as no-escape but escapes to heap
go-escape-lint: function at main.go:31 is marked as must-inline but is not inlined
```

## Examples

The annotations are placed as comments in the code and are parsed by the linter tool. 
They must be placed on the same line as the code they are annotating. 
Note that there is no space after the `//` to distinguish them from regular comments.

### `//must-inline`

The function call at the site is expected to be inlined by the compiler.
If it isn’t, the linter will produce a warning.

```go
package main

import (
	"math"
)

func foo() {
	math.Sqrt(42)
}

func bar() {
	defer func() {}()
	math.Sqrt(42)
}

func main() {
	foo() //must-inline
	bar() //must-inline // this one is not inlined and will cause a warning
}

```

### `//no-escape`

Applied to variable declarations, this ensures that the variable does not escape to the heap. 
The linter will produce a warning if the variable escapes.

```go
package main

import "math/rand"

func main() {
	_ = make([]int, 10)            //no-escape
	_ = make([]int, rand.Intn(10)) //no-escape // this one will cause a warning
}
```

### `//no-bound-check`

Applied to lines of code that access arrays or slices by index. 
The linter will produce a warning if the compiler inserts bounds checks for the access.

```go
package main

func foo(bytes []byte) {
	_ = bytes[10]
	_ = uint16(bytes[0]) | uint16(bytes[1])<<8 //no-bounds-check
}

func bar(bytes []byte) {
	_ = uint16(bytes[0]) | uint16(bytes[1])<<8 //no-bounds-check // this one will cause a warning
}

func main() {
	foo(make([]byte, 10))
	bar(make([]byte, 10))
}
```