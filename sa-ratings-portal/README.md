# S&A Ratings - Institutional Rating Portal

Standalone web portal for **S&A Ratings (Standard & Agentic Ratings)**.

## What It Includes

- Directory-style landing page: `The Registry`
- Centralized Agent ID search (`/api/agents?query=...`)
- Full reliability score to credit rating ladder
- Backend-simulated live feed: `RECENT RATING ACTIONS`
- Institutional minimal UI style: monospace, sharp corners, paper texture, high contrast

## Tech

- Frontend: vanilla HTML/CSS/JS
- Backend: Node.js + Express

## Run

```bash
npm install
npm start
```

Open `http://localhost:3000`.

## Development

```bash
npm run dev
```

## API

### `GET /api/agents?query=<string>`

Returns filtered agent records.

Sample response:

```json
{
  "query": "agt-008",
  "count": 1,
  "agents": [
    {
      "agentId": "AGT-008-SIGMA",
      "name": "Sigma Execution Proxy",
      "reliabilityScore": 0.22,
      "lastAction": "Default event opened after repeated settlement misses",
      "rating": "D"
    }
  ]
}
```

### `GET /api/events`

Returns recent simulated credit events, newest first.

Sample response:

```json
{
  "count": 5,
  "events": [
    {
      "timestamp": "2026-02-13T19:27:33.399Z",
      "agentId": "AGT-001-ALPHA",
      "score": 0.96,
      "action": "Rating affirmed",
      "note": "Operational continuity maintained for 180 days.",
      "rating": "AAA"
    }
  ]
}
```

## Rating Ladder

- `0.90 - 1.00`: `AAA`
- `0.80 - <0.90`: `AA`
- `0.70 - <0.80`: `A`
- `0.60 - <0.70`: `BBB`
- `0.50 - <0.60`: `BB`
- `0.40 - <0.50`: `B`
- `0.30 - <0.40`: `C`
- `<0.30`: `D`

## Data Seeding

- Agent seeds: `server/data/agents.js`
- Simulated events: `server/data/events.js`

You can edit those files to customize the registry and live feed behavior.

## Future Production Integration

- Replace seeded datasets with persistent storage
- Add signed event ingestion pipeline
- Introduce auth and access control for institutional users
