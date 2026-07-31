package report

import (
	"fmt"
	"slices"
	"strings"

	"github.com/fatih/color"
)

const (
	newLine         = "\n"
	emptySpace      = "    "
	middleSegment   = "├── "
	continueSegment = "│   "
	lastSegment     = "└── "
)

type (
	// Tree represents tree structure.
	Tree interface {
		AddNode(text string) Tree
		AddSubtree(tree Tree)
		Nodes() []Tree
		String() string
		Print() string
	}

	// node implements Tree.
	node struct {
		text  string
		nodes []Tree
	}

	// treePrinter traverses the tree and prints the results.
	treePrinter struct{}
)

// newNode returns a new Tree structure.
func newNode(text string) Tree {
	return &node{
		text:  text,
		nodes: []Tree{},
	}
}

// AddNode adds a node to the node.
func (t *node) AddNode(text string) Tree {
	n := newNode(text)
	t.nodes = append(t.nodes, n)
	return n
}

// AddSubtree adds a tree as an node.
func (t *node) AddSubtree(tree Tree) {
	t.nodes = append(t.nodes, tree)
}

// Text returns the node's value.
func (t *node) String() string {
	return t.text
}

// Nodes returns all nodes in the current node.
func (t *node) Nodes() []Tree {
	return t.nodes
}

// Print returns an a text representation of the node.
func (t *node) Print() string {
	return newPrinter().printTree(t)
}

// newPrinter returns a pointer to treePrinter.
func newPrinter() *treePrinter {
	return &treePrinter{}
}

// Print prints a node to a string.
func (p *treePrinter) printTree(t Tree) string {
	return t.String() + newLine + p.printNodes(t.Nodes(), []bool{})
}

func (p *treePrinter) printText(text string, spaces []bool, last bool) string {
	var result string
	for _, space := range spaces {
		if space {
			result += emptySpace
		} else {
			result += continueSegment
		}
	}

	segment := middleSegment
	if last {
		segment = lastSegment
	}

	var out string
	lines := strings.Split(text, "\n")
	for i := range lines {
		text := lines[i]
		if i == 0 {
			out += result + segment + text + newLine
			continue
		}
		if last {
			segment = emptySpace
		} else {
			segment = continueSegment
		}
		out += result + segment + text + newLine
	}

	return out
}

func (p *treePrinter) printNodes(t []Tree, spaces []bool) string {
	var result string
	for i, f := range t {
		last := i == len(t)-1
		result += p.printText(f.String(), spaces, last)
		if len(f.Nodes()) > 0 {
			spacesChild := append(spaces, last)
			result += p.printNodes(f.Nodes(), spacesChild)
		}
	}
	return result
}

// colorf renders format/a with the given color attribute, honoring the
// Builder's per-instance useColor setting instead of the fatih/color
// package-global switch.
func (b *Builder) colorf(attr color.Attribute, format string, a ...any) string {
	c := color.New(attr)
	if b.useColor {
		c.EnableColor()
	} else {
		c.DisableColor()
	}
	return c.Sprintf(format, a...)
}

// ToTree returns the report as a tree structure in text format.
func (b *Builder) ToTree() (string, error) {
	b.finalize()
	// translate slice of Sections into a map structure
	nodeMap := make(map[string]map[string]map[string]struct {
		success []string
		failed  []string
	})
	for _, section := range b.Report.Sections {
		if nodeMap[section.File] == nil {
			nodeMap[section.File] = make(map[string]map[string]struct {
				success []string
				failed  []string
			})
		}
		if nodeMap[section.File][section.Group] == nil {
			nodeMap[section.File][section.Group] = make(map[string]struct {
				success []string
				failed  []string
			})
		}

		results := nodeMap[section.File][section.Group][section.Name]

		results.success = append(results.success, section.Results...)
		results.failed = append(results.failed, section.NoResults...)

		nodeMap[section.File][section.Group][section.Name] = results
	}

	// finally build the tree, walking the maps in sorted key order so the
	// output is stable across runs (map iteration order is not).
	root := newNode(".")
	for _, file := range sortedKeys(nodeMap) {
		groups := nodeMap[file]

		// tree depth 1: files
		prefixedFile := fmt.Sprintf("%s %s", b.colorf(color.FgYellow, "%s", "[file]"), file)
		fileNode := newNode(prefixedFile)

		for _, group := range sortedKeys(groups) {
			rules := groups[group]

			// tree depth 2: groups
			groupLabel := fmt.Sprintf("%s %s", b.colorf(color.FgYellow, "%s", "[group]"), group)
			groupNode := newNode(groupLabel)

			for _, rule := range sortedKeys(rules) {
				results := rules[rule]

				// tree depth 3: rules
				prefixedRule := fmt.Sprintf(
					"%s %s",
					b.colorf(
						color.FgYellow,
						"[%d/%d]",
						len(results.success),
						len(results.success)+len(results.failed),
					),
					rule,
				)
				ruleNode := newNode(prefixedRule)

				// tree dept 4: selectors
				for _, i := range results.success {
					prefixedSuccess := b.colorf(color.FgGreen, "%s %s", "[✔]", i)
					ruleNode.AddNode(prefixedSuccess)
				}

				for _, i := range results.failed {
					prefixedFailed := b.colorf(color.FgRed, "%s %s", "[✖]", i)
					ruleNode.AddNode(prefixedFailed)
				}

				groupNode.AddSubtree(ruleNode)
			}

			fileNode.AddSubtree(groupNode)
		}

		root.AddSubtree(fileNode)
	}

	return root.Print() + b.addSummary(), nil
}

// sortedKeys returns the keys of m in sorted order, so callers can produce
// deterministic output when iterating over a map.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (b *Builder) addSummary() string {
	res := "\n"
	res += fmt.Sprintf("Groups total: %d, Rules total: %d\n", b.Report.TotalGroups, b.Report.TotalRules)
	res += fmt.Sprintf(
		"Selectors total: %d, Results found: %d, No Results found %d (No Results/Total: %.2f%%)",
		b.Report.TotalSelectorsFailed+b.Report.TotalSelectorsSuccess,
		b.Report.TotalSelectorsSuccess,
		b.Report.TotalSelectorsFailed,
		b.Report.RatioFailedTotal,
	)
	return res
}
