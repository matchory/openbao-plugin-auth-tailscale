package tsauth_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	// A guarantee is published as a heading: "### G1: A login ...".
	guaranteeHeading = regexp.MustCompile(`(?m)^### (G[0-9]+): `)
	// A test names the guarantees it proves: "// Guards: G1, G3".
	guardMarker = regexp.MustCompile(`(?m)^\s*(?://|#) Guards: (G[0-9]+(?:, G[0-9]+)*)\s*$`)
	// A marker counts only where it marks a test: in Go, the doc comment of a
	// Test or Fuzz function; in a script, a block of lines that runs a check
	// before the next blank line.
	attachedMarker = map[string]*regexp.Regexp{
		".go": regexp.MustCompile(
			`(?m)^// Guards: (G[0-9]+(?:, G[0-9]+)*)\n(?://.*\n)*func (?:Test|Fuzz)[A-Z]\w*\(`,
		),
		".sh": regexp.MustCompile(`(?m)^# Guards: (G[0-9]+(?:, G[0-9]+)*)\n(?:.+\n)*?.*\bcheck `),
	}
)

// TestEveryGuaranteeIsGuarded fails when SECURITY.md publishes a guarantee
// that no test claims to prove, or a test claims one that is not published.
// With -v it lists the files guarding each guarantee.
func TestEveryGuaranteeIsGuarded(t *testing.T) {
	t.Parallel()

	policy, err := os.ReadFile("SECURITY.md")
	if err != nil {
		t.Fatal(err)
	}

	var published []string
	for _, match := range guaranteeHeading.FindAllStringSubmatch(string(policy), -1) {
		published = append(published, match[1])
	}

	if len(published) == 0 {
		t.Fatal("SECURITY.md publishes no guarantee: no heading has the form \"### G1: ...\"")
	}

	guards := map[string][]string{}

	err = filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			if strings.HasPrefix(entry.Name(), ".") && path != "." {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(path, "_test.go") && !strings.HasSuffix(path, ".sh") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		attached := attachedMarker[filepath.Ext(path)].FindAllStringSubmatch(string(content), -1)
		if all := guardMarker.FindAllString(string(content), -1); len(all) != len(attached) {
			t.Errorf("%s: %d of its Guards markers mark no test", path, len(all)-len(attached))
		}

		for _, match := range attached {
			for id := range strings.SplitSeq(match[1], ", ") {
				if !slices.Contains(guards[id], path) {
					guards[id] = append(guards[id], path)
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, id := range published {
		if len(guards[id]) == 0 {
			t.Errorf("%s is published in SECURITY.md, but no test guards it", id)
		}

		t.Logf("%s: %s", id, strings.Join(guards[id], " "))
	}

	for id, files := range guards {
		if !slices.Contains(published, id) {
			t.Errorf(
				"%s is not a guarantee in SECURITY.md, but %s guards it",
				id,
				strings.Join(files, " "),
			)
		}
	}
}
