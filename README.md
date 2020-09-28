# olm-index-graph
Generate the upgrade graphs from an OLM index

## Install

Clone the repo, build from source, and install at `$GOBIN/olm-index-graph`

```sh
git clone git@github.com:joelanford/olm-index-graph.git
cd olm-index-graph
make install
```

## Usage

```
$ olm-index-graph -h
Generate the upgrade graphs from an OLM index

Usage:
  olm-index-graph <database file> [flags]

Flags:
  -h, --help                  help for olm-index-graph
  -p, --package-name string   Package to generate graph for (default: all)
```

## Example

Extract an OLM index database, generate the graph as a PNG (named `<dbFile>.png`), and open it with your favorite image viewer.

```sh
oc image extract quay.io/joelanford/example-operator-index:0.1.0 --file=/database/index.db
olm-index-graph index.db
```
