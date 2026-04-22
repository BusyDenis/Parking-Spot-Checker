# Parking-Spot-Checker

A small Go web app that shows nearby parking lot availability in Zurich on a map. It fetches live data from the [ParkenDD API](https://api.parkendd.de/Zuerich) and filters lots within a configurable radius of the user's location.

## Features

- Live parking data for Zurich (via ParkenDD)
- Radius-based filtering around user coordinates
- Handles API quirks (clamps impossible `Free > Total` values)
- Marks lots with no free spots as "full"
- Includes fixture lots for local testing

## Run locally

```sh
go run .
```

Then open http://localhost:3000.

Optional environment variable:
- `PORT` — port to listen on (default `3000`)

## Query parameters

- `radius` — distance in meters from the user (default `750`)
- `latitude`, `longitude` — user's coordinates (if omitted, all lots are shown)

Example: `http://localhost:3000/?latitude=47.3769&longitude=8.5417&radius=500`

## Project structure

- `main.go` — HTTP handler, API fetch, data processing
- `index.html` — Leaflet-based map template
- `go.mod` — Go module definition

## Deployment

Deployed on [deplo.io](https://deploi.io). Builds trigger automatically on push to `main`.
