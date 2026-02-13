const RECENT_EVENTS = [
  {
    timestamp: "2026-02-13T08:30:00.000Z",
    agentId: "AGT-001-ALPHA",
    score: 0.96,
    action: "Rating affirmed",
    note: "Operational continuity maintained for 180 days."
  },
  {
    timestamp: "2026-02-13T07:48:00.000Z",
    agentId: "AGT-005-ECHO",
    score: 0.58,
    action: "Rating downgraded",
    note: "Stress corridor exceedance detected in intraday cycle."
  },
  {
    timestamp: "2026-02-13T07:10:00.000Z",
    agentId: "AGT-003-GAMMA",
    score: 0.74,
    action: "Outlook revised",
    note: "Liquidity routing confidence improved after patch release."
  },
  {
    timestamp: "2026-02-13T06:42:00.000Z",
    agentId: "AGT-008-SIGMA",
    score: 0.22,
    action: "Default action opened",
    note: "Repeated settlement breach beyond tolerance threshold."
  },
  {
    timestamp: "2026-02-13T06:08:00.000Z",
    agentId: "AGT-002-BETA",
    score: 0.83,
    action: "Watchlist removed",
    note: "Collateral sufficiency restored following reconciliation."
  }
];

function buildSimulatedEvents() {
  const now = Date.now();
  return RECENT_EVENTS.map((item, index) => {
    const shiftedTimestamp = new Date(now - (index + 1) * 210000).toISOString();
    return { ...item, timestamp: shiftedTimestamp };
  });
}

module.exports = { buildSimulatedEvents };
