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

Extract an OLM index database, convert the graph database to a .dot graphviz file (named `<dbFile>.dot`), and render it:

```sh
IMAGE=quay.io/operator-framework/upstream-community-operators:latest
ID=$(docker create $IMAGE)
docker cp $ID:/bundles.db upstream-community-operators.db && docker rm $ID
olm-index-graph upstream-community-operators.db
dot upstream-community-operators.db.dot -Tsvg -o upstream-community-operators.db.svg
```
