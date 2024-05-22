package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseCompilerOutput(t *testing.T) {
	// Create a temporary directory.
	tmpDir := t.TempDir()

	// Create a temporary file with compiler output.
	compilerOutput := `
main.go:10: moved to heap: main
main.go:15: escapes to heap: main
main.go:20: stays on stack: main
main.go:25: inlining call: main
main.go:30: Found IsInBounds
`
	tmpFile := filepath.Join(tmpDir, "compiler_output.txt")
	if err := os.WriteFile(tmpFile, []byte(compilerOutput), 0644); err != nil {
		t.Fatalf("failed to write to temp file: %v", err)
	}

	// Call ParseCompilerOutput with the temp file.
	results, err := ParseCompilerOutput(tmpFile)
	if err != nil {
		t.Fatalf("ParseCompilerOutput failed: %v", err)
	}

	// Create expected results with normalized paths.
	expected := map[Position][]CompilerHint{
		{File: filepath.Join(tmpDir, "main.go"), Line: 10}: {MovedToHeap},
		{File: filepath.Join(tmpDir, "main.go"), Line: 15}: {EscapesToHeap},
		{File: filepath.Join(tmpDir, "main.go"), Line: 20}: {StaysOnStack},
		{File: filepath.Join(tmpDir, "main.go"), Line: 25}: {Inlined},
		{File: filepath.Join(tmpDir, "main.go"), Line: 30}: {FoundIsInBounds},
	}

	if !reflect.DeepEqual(results, expected) {
		t.Errorf("expected %v, got %v", expected, results)
	}
}

func TestParseCodeAnnotations(t *testing.T) {
	tmpDir := t.TempDir()

	mainGo := `
package main

func main() {
	var a int //no-escape
	var b int //no-bounds-check
	var c int //must-inline
}
`
	mainGoFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(mainGoFile, []byte(mainGo), 0644); err != nil {
		t.Fatalf("failed to write to main.go: %v", err)
	}

	results, valid, err := ParseCodeAnnotations(tmpDir)
	if err != nil {
		t.Fatalf("ParseCodeAnnotations failed: %v", err)
	}

	expected := map[Position][]Annotation{
		{File: mainGoFile, Line: 5}: {NoEscape},
		{File: mainGoFile, Line: 6}: {NoBoundsCheck},
		{File: mainGoFile, Line: 7}: {MustInline},
	}

	if !reflect.DeepEqual(results, expected) {
		t.Errorf("expected %v, got %v", expected, results)
	}

	if !valid {
		t.Errorf("expected annotations to be valid")
	}
}

func TestCompareResults(t *testing.T) {
	tests := []struct {
		name            string
		compilerHints   map[Position][]CompilerHint
		codeAnnotations map[Position][]Annotation
		expectedValid   bool
	}{
		{
			name: "validCases",
			compilerHints: map[Position][]CompilerHint{
				{File: "main.go", Line: 10}: {StaysOnStack},
				{File: "main.go", Line: 15}: {StaysOnStack},
				{File: "main.go", Line: 25}: {Inlined},
			},
			codeAnnotations: map[Position][]Annotation{
				{File: "main.go", Line: 10}: {NoEscape},
				{File: "main.go", Line: 15}: {NoEscape},
				{File: "main.go", Line: 20}: {NoBoundsCheck},
				{File: "main.go", Line: 25}: {MustInline},
			},
			expectedValid: true,
		},
		{
			name: "invalidNoEscape",
			compilerHints: map[Position][]CompilerHint{
				{File: "main.go", Line: 10}: {EscapesToHeap},
				{File: "main.go", Line: 15}: {MovedToHeap},
				{File: "main.go", Line: 20}: {StaysOnStack},
				{File: "main.go", Line: 25}: {Inlined},
			},
			codeAnnotations: map[Position][]Annotation{
				{File: "main.go", Line: 10}: {NoEscape},
				{File: "main.go", Line: 15}: {NoEscape},
				{File: "main.go", Line: 20}: {NoBoundsCheck},
				{File: "main.go", Line: 25}: {MustInline},
			},
			expectedValid: false,
		},
		{
			name: "invalidNoBoundsCheck",
			compilerHints: map[Position][]CompilerHint{
				{File: "main.go", Line: 10}: {StaysOnStack},
				{File: "main.go", Line: 15}: {StaysOnStack},
				{File: "main.go", Line: 20}: {FoundIsInBounds},
				{File: "main.go", Line: 25}: {Inlined},
			},
			codeAnnotations: map[Position][]Annotation{
				{File: "main.go", Line: 10}: {NoEscape},
				{File: "main.go", Line: 15}: {NoEscape},
				{File: "main.go", Line: 20}: {NoBoundsCheck},
				{File: "main.go", Line: 25}: {MustInline},
			},
			expectedValid: false,
		},
		{
			name: "invalidMustInline",
			compilerHints: map[Position][]CompilerHint{
				{File: "main.go", Line: 10}: {StaysOnStack},
				{File: "main.go", Line: 15}: {StaysOnStack},
				{File: "main.go", Line: 20}: {StaysOnStack},
				{File: "main.go", Line: 25}: {StaysOnStack},
			},
			codeAnnotations: map[Position][]Annotation{
				{File: "main.go", Line: 10}: {NoEscape},
				{File: "main.go", Line: 15}: {NoEscape},
				{File: "main.go", Line: 20}: {NoBoundsCheck},
				{File: "main.go", Line: 25}: {MustInline},
			},
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := CompareResults(tt.compilerHints, tt.codeAnnotations)
			if valid != tt.expectedValid {
				t.Fatalf("expected %v, got %v", tt.expectedValid, valid)
			}
		})
	}
}
