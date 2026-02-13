const searchInput = document.getElementById("agent-search");
const searchButton = document.getElementById("search-btn");
const registryBody = document.getElementById("registry-body");
const registryCount = document.getElementById("registry-count");
const registryEmpty = document.getElementById("registry-empty");
const feedList = document.getElementById("feed-list");
const feedLastUpdated = document.getElementById("feed-last-updated");
const feedScroll = document.querySelector(".feed-scroll");

function mapScoreToRating(score) {
  if (score >= 0.9) return "AAA";
  if (score >= 0.8) return "AA";
  if (score >= 0.7) return "A";
  if (score >= 0.6) return "BBB";
  if (score >= 0.5) return "BB";
  if (score >= 0.4) return "B";
  if (score >= 0.3) return "C";
  return "D";
}

function formatTime(iso) {
  const date = new Date(iso);
  return date.toLocaleString("en-GB", {
    hour12: false,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit"
  });
}

function renderRegistry(agents) {
  registryBody.innerHTML = "";
  registryCount.textContent = `${agents.length} entr${agents.length === 1 ? "y" : "ies"}`;
  registryEmpty.classList.toggle("hidden", agents.length !== 0);

  for (const agent of agents) {
    const row = document.createElement("tr");
    const rating = agent.rating || mapScoreToRating(agent.reliabilityScore);
    row.innerHTML = `
      <td>${agent.agentId}</td>
      <td>${agent.name}</td>
      <td>${agent.reliabilityScore.toFixed(2)}</td>
      <td><span class="rating-chip">${rating}</span></td>
      <td>${agent.lastAction}</td>
    `;
    registryBody.appendChild(row);
  }
}

function renderEvents(events) {
  feedList.innerHTML = "";
  for (const event of events) {
    const item = document.createElement("li");
    item.innerHTML = `
      <div class="event-line">
        <strong>${event.agentId}</strong>
        <span class="rating-chip">${event.rating || mapScoreToRating(event.score)}</span>
        <span>${event.action}</span>
      </div>
      <div class="event-line event-time">${formatTime(event.timestamp)}</div>
      <div>${event.note}</div>
    `;
    feedList.appendChild(item);
  }
  feedLastUpdated.textContent = `last sync ${formatTime(new Date().toISOString())}`;
}

async function loadAgents(query = "") {
  const response = await fetch(`/api/agents?query=${encodeURIComponent(query)}`);
  if (!response.ok) {
    throw new Error("Failed to load agents.");
  }
  const payload = await response.json();
  renderRegistry(payload.agents || []);
}

async function loadEvents() {
  const response = await fetch("/api/events");
  if (!response.ok) {
    throw new Error("Failed to load events.");
  }
  const payload = await response.json();
  renderEvents(payload.events || []);
}

function smoothAutoScroll() {
  if (!feedScroll || feedScroll.scrollHeight <= feedScroll.clientHeight) {
    return;
  }

  feedScroll.scrollTop += 1;
  if (feedScroll.scrollTop + feedScroll.clientHeight >= feedScroll.scrollHeight) {
    feedScroll.scrollTop = 0;
  }
}

async function refreshAll() {
  try {
    await Promise.all([loadAgents(searchInput.value.trim()), loadEvents()]);
  } catch (error) {
    console.error(error);
  }
}

searchButton.addEventListener("click", () => {
  loadAgents(searchInput.value.trim()).catch((error) => console.error(error));
});

searchInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") {
    loadAgents(searchInput.value.trim()).catch((error) => console.error(error));
  }
});

refreshAll();
setInterval(() => {
  loadEvents().catch((error) => console.error(error));
}, 4000);
setInterval(smoothAutoScroll, 60);
