# 🐐 Dominions 6 API by Monkeydew

A fast in-memory **SQLite-powered API** for _Dominions 6 Inspector_ data.  
Built with Go.  
All game data (units, items, spells, sites, mercs, events), caputres of their UIs as an exposed queryable JSON REST API.

---

## Launch Modes

Uses Fetched data from the [dom6inspector GitHub](https://github.com/larzm42/dom6inspector) repo, runs a local web server, stored in SQLite.

_Note:_
If you don't have go installed yet, you can install it with:
Mac: `brew install go`
Linux: `sudo apt install golang-go`
Windows: https://go.dev/dl/ (and follow the instructions)

**No-Build Mode (API only):**
This will skip scraping and launches only the API using existing data.
`dom6api.exe`

Outputs logs like:

16:37:51 | 200 | 2.5538ms | 127.0.0.1 | GET | /items/1 | -

Each table mirrors the inspector data:

Table Columns
items id, name, type, constlevel, mainlevel, mpath, gemcost
spells id, name, gemcost, mpath, type, school, researchlevel
units id, name, hp, size, mountmnr, coridermnr
mercs id, name, bossname, com, unit, nrunits
sites id, name, path, level, rarity
events id, name

All tables also include a generated image path, e.g.
Data/spells/123.png

## API Usage

Query by ID
`/spells/1`
`/spells/?id=123`

Query by Column
`/units/?name=archer`

Fuzzy Search (enable fuzzy and partial matching):
`/spells/?name=bless&match=fuzzy`
Matches even if names differ slightly

Images:  
`table/id/screenshot`
returns image

## Example Response

```
{
    "spells": [{
        "id": 42,
        "name": "Bless",
        "school": "Enchantment",
        "gemcost": 1,
        "image": "Data/spells/42.png"
    }]
}
```

## API Usage

To scrape: cd into scrape and run "go run ." it does the rest.
Right now the copy from scrape to api is manual

## Compile

Install required dependencies with:
`go mod tidy`

Windows:
`go build -trimpath -ldflags="-s -w" -o dom6api.exe .`

Linux (Ubuntu):
`set GOOS=linux`
`set GOARCH=amd64`
`go build -trimpath -ldflags="-s -w" -o dom6api_linux .`

macOS:
`set GOOS=darwin`
`set GOARCH=amd64`
`go build -trimpath -ldflags="-s -w" -o dom6api_mac .`

## Warning ⚠️

This software was built and tested by an idiot.

## 🐐 Credits 🐐

Big thanks to Tim Nordenfur for [dom5api](https://github.com/gtim/dom5api) — a huge source of inspiration and the defacto predecessor.  
Big thanks for Toldi in assistance with the process.

Created by Monkeydew.
Use, modify, and distribute freely.
MIT Licensed.
