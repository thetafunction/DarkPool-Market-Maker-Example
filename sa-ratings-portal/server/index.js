const express = require("express");
const path = require("path");

const { mapScoreToRating } = require("./rating");
const { AGENTS } = require("./data/agents");
const { buildSimulatedEvents } = require("./data/events");

const app = express();
const PORT = process.env.PORT || 3000;
const publicDir = path.join(__dirname, "..", "public");

app.use(express.json());
app.use(express.static(publicDir));

function withRating(agent) {
  return {
    ...agent,
    rating: mapScoreToRating(agent.reliabilityScore)
  };
}

app.get("/api/agents", (req, res) => {
  const query = String(req.query.query || "").trim().toLowerCase();
  const base = AGENTS.map(withRating);

  const filtered = query
    ? base.filter((agent) => {
        return (
          agent.agentId.toLowerCase().includes(query) ||
          agent.name.toLowerCase().includes(query)
        );
      })
    : base;

  res.json({
    query,
    count: filtered.length,
    agents: filtered
  });
});

app.get("/api/events", (_req, res) => {
  const events = buildSimulatedEvents()
    .map((event) => ({ ...event, rating: mapScoreToRating(event.score) }))
    .sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

  res.json({
    count: events.length,
    events
  });
});

app.get(/.*/, (_req, res) => {
  res.sendFile(path.join(publicDir, "index.html"));
});

app.listen(PORT, () => {
  // eslint-disable-next-line no-console
  console.log(`S&A Ratings portal listening on http://localhost:${PORT}`);
});
