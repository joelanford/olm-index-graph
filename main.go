package main

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"

	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

type pkg struct {
	graph    *cgraph.Graph
	channels map[string]*cgraph.Graph
}

type channelEntry struct {
	packageName        string
	channelName        string
	bundleName         string
	replacesBundleName string
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
	channelEntries, err := loadEntries(dbFile, packageName)
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

	if err := populateIndexGraph(graph, channelEntries); err != nil {
		return err
	}

	pngFile := filepath.Join(filepath.Dir(dbFile), fmt.Sprintf("%s.png", filepath.Base(dbFile)))
	if err := g.RenderFilename(graph, graphviz.PNG, pngFile); err != nil {
		return err
	}
	return nil
}

const entriesSQL = "SELECT e.package_name,e.channel_name,e.operatorbundle_name,r.operatorbundle_name FROM channel_entry e INNER JOIN channel_entry r ON r.entry_id = e.replaces UNION SELECT package_name,channel_name,operatorbundle_name,'' FROM channel_entry WHERE replaces IS NULL"

func loadEntries(dbFile, packageName string) ([]channelEntry, error) {
	db, err := sql.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}

	entryRows, err := db.Query(entriesSQL)
	if err != nil {
		return nil, err
	}
	defer entryRows.Close()
	entries := make([]channelEntry, 0)
	for entryRows.Next() {
		var e channelEntry
		if err := entryRows.Scan(&e.packageName, &e.channelName, &e.bundleName, &e.replacesBundleName); err != nil {
			return nil, err
		}
		if packageName != "" && packageName != e.packageName {
			continue
		}
		entries = append(entries, e)
	}
	if err := entryRows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func populateIndexGraph(graph *cgraph.Graph, channelEntries []channelEntry) error {
	packages := map[string]pkg{}
	for _, e := range channelEntries {
		p, ok := packages[e.packageName]
		if !ok {
			p = pkg{
				graph:    graph.SubGraph(fmt.Sprintf("cluster_%s", e.packageName), 1),
				channels: make(map[string]*cgraph.Graph),
			}
			p.graph.SetLabel(fmt.Sprintf("package: %s", e.packageName))
		}

		c, ok := p.channels[e.channelName]
		if !ok {
			c = p.graph.SubGraph(fmt.Sprintf("cluster_%s", e.channelName), 1)
			c.SetLabel(fmt.Sprintf("channel: %s", e.channelName))
		}

		from, err := c.CreateNode(fmt.Sprintf("%s_%s", e.channelName, e.bundleName))
		if err != nil {
			return err
		}
		from.SetLabel(e.bundleName)
		if e.replacesBundleName != "" {
			to, err := c.CreateNode(fmt.Sprintf("%s_%s", e.channelName, e.replacesBundleName))
			if err != nil {
				return err
			}
			to.SetLabel(e.replacesBundleName)
			if _, err := c.CreateEdge("", from, to); err != nil {
				return err
			}
		}
	}
	return nil
}
