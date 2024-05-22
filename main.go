package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Annotation string

const (
	NoEscape      Annotation = "no-escape"
	NoBoundsCheck Annotation = "no-bounds-check"
	MustInline    Annotation = "must-inline"
)

type CompilerHint string

const (
	EscapesToHeap   CompilerHint = "escapes-to-heap"
	MovedToHeap     CompilerHint = "moved-to-heap"
	StaysOnStack    CompilerHint = "stays-on-stack"
	FoundIsInBounds CompilerHint = "found-is-in-bounds"
	Inlined         CompilerHint = "inlined"
)

var knownAnnotations = []Annotation{
	NoEscape,
	NoBoundsCheck,
	MustInline,
}

const (
	logPrefix            = "go-escape-lint: "
	maxCommentLength     = 20
	levenshteinThreshold = 3
)

type Position struct {
	File string
	Line int
}

func levenshteinDistance(a, b string) int {
	if len(a) < len(b) {
		a, b = b, a
	}

	previous := make([]int, len(b)+1)
	for i := range previous {
		previous[i] = i
	}

	for i, ra := range a {
		current := make([]int, len(b)+1)
		current[0] = i + 1

		for j, rb := range b {
			insertions := previous[j+1] + 1
			deletions := current[j] + 1
			substitutions := previous[j]

			if ra != rb {
				substitutions++
			}

			current[j+1] = min(insertions, deletions, substitutions)
		}

		previous = current
	}

	return previous[len(b)]
}

func ParseCompilerOutput(filePath string) (map[Position][]CompilerHint, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	results := make(map[Position][]CompilerHint)
	scanner := bufio.NewScanner(file)
	dirname := path.Dir(filePath)
	scannerLine := 1

	for scanner.Scan() {
		var annotation CompilerHint
		line := scanner.Text()

		switch {
		case strings.Contains(line, "escapes to heap"):
			annotation = EscapesToHeap
		case strings.Contains(line, "moved to heap"):
			annotation = MovedToHeap
		case strings.Contains(line, "stays on stack"):
			annotation = StaysOnStack
		case strings.Contains(line, "inlining call"):
			annotation = Inlined
		case strings.Contains(line, "Found IsInBounds"):
			annotation = FoundIsInBounds
		}

		if annotation != "" {
			parts := strings.Fields(line)

			if len(parts) > 0 {
				pos := strings.Split(parts[0], ":")

				if len(pos) >= 2 {
					lineNum, err := strconv.Atoi(pos[1])
					if err != nil {
						return nil, fmt.Errorf("failed to parse line number at %d: %w", scannerLine, err)
					}

					fileName := pos[0]
					normalizedFile := path.Clean(path.Join(dirname, fileName))
					lineKey := Position{File: normalizedFile, Line: lineNum}
					results[lineKey] = append(results[lineKey], annotation)
				}
			}
		}

		scannerLine++
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func splitLine(line string) (code, comment string) {
	if i := strings.Index(line, "//"); i != -1 {
		return strings.TrimSpace(line[:i]), strings.TrimSpace(line[i:])
	}

	return strings.TrimSpace(line), ""
}

func ParseCodeAnnotations(packagePath string) (map[Position][]Annotation, bool, error) {
	annotations := make(map[Position][]Annotation)

	var (
		noEscape      = fmt.Sprintf("//%s", NoEscape)
		noBoundsCheck = fmt.Sprintf("//%s", NoBoundsCheck)
		mustInline    = fmt.Sprintf("//%s", MustInline)
		valid         = true
	)

	err := filepath.Walk(packagePath, func(currentPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and vendor
		if info.IsDir() && info.Name() != "." && (strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor") {
			return filepath.SkipDir
		}

		// Skip non-Go files
		if info.IsDir() || !strings.HasSuffix(currentPath, ".go") {
			return nil
		}

		// Skip test files
		if strings.HasSuffix(currentPath, "_test.go") {
			return nil
		}

		file, err := os.Open(currentPath)
		if err != nil {
			return err
		}

		defer func() {
			_ = file.Close()
		}()

		scanner := bufio.NewScanner(file)
		lineNum := 0

		for scanner.Scan() {
			lineNum++

			line := scanner.Text()
			code, comment := splitLine(line)
			var lineAnnotations []Annotation

			if code == "" || comment == "" {
				continue
			}

			switch {
			case strings.Contains(comment, noEscape):
				lineAnnotations = append(lineAnnotations, NoEscape)
			case strings.Contains(comment, noBoundsCheck):
				lineAnnotations = append(lineAnnotations, NoBoundsCheck)
			case strings.Contains(comment, mustInline):
				lineAnnotations = append(lineAnnotations, MustInline)
			}

			if len(lineAnnotations) > 0 {
				normalizedFile := path.Clean(currentPath)
				lineKey := Position{File: normalizedFile, Line: lineNum}
				annotations[lineKey] = append(annotations[lineKey], lineAnnotations...)
			}

			// We haven't found any annotations, but there is some suspicious comment.
			// Let’s check if this might be an annotation with a typo.
			if len(lineAnnotations) == 0 && len(comment) <= maxCommentLength {
				for _, ann := range knownAnnotations {
					if levenshteinDistance(comment, string(ann)) <= levenshteinThreshold {
						log.Printf("probably a typo '%s' at %s:%d", comment, currentPath, lineNum)
						valid = false
					}
				}
			}
		}

		if err := scanner.Err(); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, valid, err
	}

	return annotations, valid, nil
}

func CompareResults(
	compilerHints map[Position][]CompilerHint,
	codeAnnotations map[Position][]Annotation,
) (valid bool) {
	valid = true

	for pos, annotations := range codeAnnotations {
		hints := compilerHints[pos]

		for _, ann := range annotations {
			switch ann {
			case NoEscape:
				if slices.Contains(hints, EscapesToHeap) || slices.Contains(hints, MovedToHeap) {
					log.Printf("variable at %s:%d is marked as %s but escapes to heap", pos.File, pos.Line, ann)
					valid = false
				}
			case NoBoundsCheck:
				if slices.Contains(hints, FoundIsInBounds) {
					log.Printf("variable at %s:%d is marked as %s but bounds check is not eliminated", pos.File, pos.Line, ann)
					valid = false
				}
			case MustInline:
				if !slices.Contains(hints, Inlined) {
					log.Printf("function at %s:%d is marked as %s but is not inlined", pos.File, pos.Line, ann)
					valid = false
				}
			}
		}
	}

	return valid
}

type Options struct {
	Pkg       string
	InputFile string
	NoFail    bool
}

func parseOptions() Options {
	opts := Options{}
	flag.BoolVar(&opts.NoFail, "no-fail", false, "Exit with status code 0 even if errors are found")
	flag.StringVar(&opts.InputFile, "f", "", "Path to the compiler output file")
	flag.StringVar(&opts.Pkg, "pkg", ".", "Path to the package directory")
	flag.Parse()

	if opts.InputFile == "" {
		log.Println("error: compiler output file is required")
		flag.Usage()
		os.Exit(1)
	}

	return opts
}

func main() {
	opts := parseOptions()

	log.SetPrefix(logPrefix)
	log.SetOutput(os.Stdout)
	log.SetFlags(0)

	hints, err := ParseCompilerOutput(opts.InputFile)
	if err != nil {
		log.Fatalf("error parsing compiler output: %s", err)
	}

	annotations, annotationsValid, err := ParseCodeAnnotations(opts.Pkg)
	if err != nil {
		log.Fatalf("error parsing source code: %s", err)
	}

	resultValid := CompareResults(hints, annotations)

	if (!annotationsValid || !resultValid) && !opts.NoFail {
		os.Exit(1)
	}
}
