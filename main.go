package main

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/blang/semver/v4"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/util/sets"
)

type channelEntry struct {
	packageName        string
	channelName        string
	bundleName         string
	depth              int
	bundleVersion      *string
	bundleSkipRange    *string
	replacesBundleName *string
}

type pkg struct {
	name    string
	bundles map[string]*bundle
}

type bundle struct {
	name              string
	version           string
	packageName       string
	skipRange         string
	minDepth          int
	channels          sets.String
	replaces          sets.String
	skipRangeReplaces sets.String
	isBundlePresent   bool
}

func main() {
	var packageName string
	root := cobra.Command{
		Use:   "olm-index-graph <database file>",
		Short: "Generate the upgrade graphs from an OLM index",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			dbFile := args[0]
			return run(dbFile, packageName)
		},
	}

	root.Flags().StringVarP(&packageName, "package-name", "p", "", "Package to generate graph for (default: all)")

	if err := root.Execute(); err != nil {
		log.Fatal(err)
	}
}

func run(dbFile, packageName string) error {
	pkgs, err := loadPackages(dbFile, packageName)
	if err != nil {
		return err
	}

	g := graphviz.New()
	defer g.Close()

	graph, err := g.Graph()
	if err != nil {
		return err
	}

	defer graph.Close()

	if err := populateIndexGraph(graph, pkgs); err != nil {
		return err
	}

	dotFile := filepath.Join(filepath.Dir(dbFile), fmt.Sprintf("%s.dot", filepath.Base(dbFile)))
	if err := g.RenderFilename(graph, graphviz.XDOT, dotFile); err != nil {
		return err
	}
	return nil
}

const entriesSQL = `
SELECT
	e.package_name,
	e.channel_name,
	e.operatorbundle_name,
	e.depth,
	b.version,
	b.skipRange,
	r.operatorbundle_name
FROM
	channel_entry e
LEFT JOIN
	channel_entry r ON r.entry_id = e.replaces
LEFT JOIN
	operatorbundle b ON e.operatorbundle_name = b.name
`

func loadPackages(dbFile, packageName string) (map[string]*pkg, error) {
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}

	pkgs := map[string]*pkg{}

	entryRows, err := db.Query(entriesSQL)
	if err != nil {
		return nil, err
	}
	defer entryRows.Close()
	for entryRows.Next() {
		var e channelEntry
		if err := entryRows.Scan(&e.packageName, &e.channelName, &e.bundleName, &e.depth, &e.bundleVersion, &e.bundleSkipRange, &e.replacesBundleName); err != nil {
			return nil, err
		}
		if packageName != "" && packageName != e.packageName {
			continue
		}

		// Get or create package
		p, ok := pkgs[e.packageName]
		if !ok {
			p = &pkg{
				name:    e.packageName,
				bundles: make(map[string]*bundle),
			}
		}
		pkgs[e.packageName] = p

		// Get or create bundle
		b, ok := p.bundles[e.bundleName]
		if !ok {
			b = &bundle{
				name:              e.bundleName,
				packageName:       e.packageName,
				minDepth:          e.depth,
				isBundlePresent:   e.bundleVersion != nil,
				channels:          sets.NewString(),
				replaces:          sets.NewString(),
				skipRangeReplaces: sets.NewString(),
			}
			if e.bundleSkipRange != nil {
				b.skipRange = *e.bundleSkipRange
			}
			if e.bundleVersion != nil {
				b.version = *e.bundleVersion
			}
		}
		p.bundles[e.bundleName] = b

		b.channels.Insert(e.channelName)
		if e.replacesBundleName != nil {
			b.replaces.Insert(*e.replacesBundleName)
		}
		if e.depth < b.minDepth {
			b.minDepth = e.depth
		}

	}
	for _, p := range pkgs {
		for _, pb := range p.bundles {
			if pb.skipRange == "" {
				continue
			}
			pSkipRange, err := semver.ParseRange(pb.skipRange)
			if err != nil {
				return nil, fmt.Errorf("invalid range %q for bundle %q: %v", pb.skipRange, pb.name, err)
			}
			for _, cb := range p.bundles {
				if !cb.isBundlePresent {
					continue
				}
				cVersion, err := semver.Parse(cb.version)
				if err != nil {
					return nil, fmt.Errorf("invalid version %q for bundle %q: %v", cb.version, cb.name, err)
				}
				if pSkipRange(cVersion) {
					pb.skipRangeReplaces.Insert(cb.name)
				}
			}
		}
	}
	if err := entryRows.Err(); err != nil {
		return nil, err
	}

	return pkgs, nil
}

func populateIndexGraph(graph *cgraph.Graph, pkgs map[string]*pkg) error {
	for _, p := range pkgs {
		pGraph := graph.SubGraph(fmt.Sprintf("cluster_%s", p.name), 1)
		pGraph.SetLabel(fmt.Sprintf("package: %s", p.name))

		for _, b := range p.bundles {
			nodeName := fmt.Sprintf("%s_%s", p.name, b.name)
			node, err := pGraph.CreateNode(nodeName)
			if err != nil {
				return err
			}
			node.SetShape("record")
			node.SetWidth(4)
			node.SetLabel(fmt.Sprintf("{%s|{channels|{%s}}}", b.name, strings.Join(b.channels.List(), "|")))
			if !b.isBundlePresent {
				node.SetStyle(cgraph.DashedNodeStyle)
			}
			if b.minDepth == 0 {
				node.SetPenWidth(4.0)
			}
		}

		for _, pb := range p.bundles {
			pName := fmt.Sprintf("%s_%s", p.name, pb.name)
			parent, err := pGraph.Node(pName)
			if err != nil {
				return err
			}
			for _, cb := range pb.replaces.List() {
				cName := fmt.Sprintf("%s_%s", p.name, cb)
				child, err := pGraph.Node(cName)
				if err != nil {
					return err
				}
				edgeName := fmt.Sprintf("replaces_%s_%s", parent.Name(), child.Name())
				if _, err := pGraph.CreateEdge(edgeName, parent, child); err != nil {
					return err
				}
			}
			for _, cb := range pb.skipRangeReplaces.List() {
				cName := fmt.Sprintf("%s_%s", p.name, cb)
				child, err := pGraph.Node(cName)
				if err != nil {
					return err
				}

				edgeName := fmt.Sprintf("skipRange_%s_%s", parent.Name(), child.Name())
				if !pb.replaces.Has(cb) {
					edge, err := pGraph.CreateEdge(edgeName, parent, child)
					if err != nil {
						return err
					}
					edge.SetStyle(cgraph.DashedEdgeStyle)
				}
			}
		}
	}
	return nil
}
