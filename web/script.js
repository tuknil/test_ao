const sidebar = document.getElementById('sidebar');
const collapseBtn = document.getElementById('collapseBtn');
const expandBtn = document.getElementById('expandBtn');

collapseBtn.addEventListener('click', () => {
  sidebar.classList.add('collapsed');
  expandBtn.classList.add('visible');
});

expandBtn.addEventListener('click', () => {
  sidebar.classList.remove('collapsed');
  expandBtn.classList.remove('visible');
});

document.querySelectorAll('.section-header').forEach((header) => {
  header.addEventListener('click', () => {
    const id = header.getAttribute('data-section');
    const list = document.getElementById(id);
    const expanded = header.getAttribute('aria-expanded') === 'true';

    header.setAttribute('aria-expanded', String(!expanded));
    list.classList.toggle('collapsed', expanded);
  });
});

// View switching
const views = {
  default: document.getElementById('defaultView'),
  integrations: document.getElementById('integrationsView'),
  agents: document.getElementById('agentsView'),
  policies: document.getElementById('policiesView'),
  policySinks: document.getElementById('policySinksView'),
  telemetrySources: document.getElementById('telemetrySourcesView'),
  signalSinks: document.getElementById('signalSinksView'),
};

function showView(name) {
  Object.entries(views).forEach(([key, el]) => {
    el.hidden = key !== name;
  });
}

const integrationsAiAssetsLink = document.getElementById('integrationsAiAssetsLink');
integrationsAiAssetsLink.addEventListener('click', (e) => {
  e.preventDefault();
  showView('integrations');
});

const agentsLink = document.getElementById('agentsLink');
agentsLink.addEventListener('click', (e) => {
  e.preventDefault();
  showView('agents');
  loadAgents();
});

const policiesLink = document.getElementById('policiesLink');
policiesLink.addEventListener('click', (e) => {
  e.preventDefault();
  showView('policies');
  loadPolicies();
});

const policySinksLink = document.getElementById('policySinksLink');
policySinksLink.addEventListener('click', (e) => {
  e.preventDefault();
  showView('policySinks');
});

const telemetrySourcesLink = document.getElementById('telemetrySourcesLink');
telemetrySourcesLink.addEventListener('click', (e) => {
  e.preventDefault();
  showView('telemetrySources');
});

const signalSinksLink = document.getElementById('signalSinksLink');
signalSinksLink.addEventListener('click', (e) => {
  e.preventDefault();
  showView('signalSinks');
});

// Wiz integrations: a reusable "add" template card plus one card per created instance
const API_BASE = '/api/wiz-integrations';

const wizCardGrid = document.getElementById('wizCardGrid');
const wizTableWrap = document.getElementById('wizTableWrap');
const wizTableBody = document.getElementById('wizTableBody');
const wizModalOverlay = document.getElementById('wizModalOverlay');
const wizModalTitle = document.getElementById('wizModalTitle');
const wizModalClose = document.getElementById('wizModalClose');
const wizCancelBtn = document.getElementById('wizCancelBtn');
const wizForm = document.getElementById('wizForm');
const wizNameInput = document.getElementById('wizName');
const wizBaseUrlInput = document.getElementById('wizBaseUrl');
const wizClientIdInput = document.getElementById('wizClientId');
const wizClientSecretInput = document.getElementById('wizClientSecret');
const wizMcpServerInput = document.getElementById('wizMcpServer');

const INTEGRATION_TEMPLATE_TYPES = ['Wiz', 'Astra', 'Datalock'];

let wizIntegrations = []; // [{ id, name, type, baseUrl, clientId, hasSecret, ... }, ...]
let editingId = null; // id being edited, or null when creating

function renderWizCard() {
  wizCardGrid.innerHTML = '';

  INTEGRATION_TEMPLATE_TYPES.forEach((type) => {
    const templateCard = document.createElement('div');
    templateCard.className = 'integration-card is-empty';
    templateCard.tabIndex = 0;
    templateCard.setAttribute('role', 'button');
    templateCard.innerHTML = `
      <span class="integration-logo">${escapeHtml(type.slice(0, 4).toUpperCase())}</span>
      <span class="integration-name">${escapeHtml(type)}</span>
      <span class="integration-base-url">Click to add an integration</span>
    `;
    templateCard.addEventListener('click', () => openWizModal('create', null, type));
    templateCard.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        openWizModal('create', null, type);
      }
    });
    wizCardGrid.appendChild(templateCard);
  });

  wizTableBody.innerHTML = '';
  wizTableWrap.hidden = wizIntegrations.length === 0;

  wizIntegrations.forEach((wi) => {
    const row = document.createElement('tr');
    row.innerHTML = `
      <td>${escapeHtml(wi.name)}</td>
      <td><span class="type-badge">${escapeHtml(wi.type)}</span></td>
      <td>
        <div class="row-actions">
          <button type="button" class="integration-action-btn" data-action="edit">Edit</button>
          <button type="button" class="integration-action-btn danger" data-action="delete">Delete</button>
        </div>
      </td>
    `;
    row.querySelector('[data-action="edit"]').addEventListener('click', () => openWizModal('edit', wi));
    row.querySelector('[data-action="delete"]').addEventListener('click', () => handleDeleteWiz(wi));
    wizTableBody.appendChild(row);
  });
}

function escapeHtml(str) {
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

async function loadWizIntegrations() {
  try {
    const res = await fetch(API_BASE);
    if (!res.ok) throw new Error('Failed to load integrations');
    wizIntegrations = await res.json();
  } catch (err) {
    console.error(err);
    wizIntegrations = [];
  }
  renderWizCard();
}

const wizTypeField = document.getElementById('wizTypeField');

function openWizModal(mode, wi, type) {
  if (mode === 'edit' && wi) {
    editingId = wi.id;
    wizModalTitle.textContent = `Edit ${wi.type} integration`;
    wizTypeField.value = wi.type;
    wizNameInput.value = wi.name;
    wizBaseUrlInput.value = wi.baseUrl;
    wizClientIdInput.value = wi.clientId;
    wizClientSecretInput.value = '';
    wizClientSecretInput.required = false;
    wizClientSecretInput.placeholder = 'Leave blank to keep existing secret';
    wizMcpServerInput.value = wi.mcpServer || '';
  } else {
    editingId = null;
    wizModalTitle.textContent = `New ${type} integration`;
    wizForm.reset();
    wizTypeField.value = type;
    wizClientSecretInput.required = true;
    wizClientSecretInput.placeholder = '';
  }
  wizModalOverlay.hidden = false;
  wizNameInput.focus();
}

function closeWizModal() {
  wizModalOverlay.hidden = true;
  wizForm.reset();
}

wizModalClose.addEventListener('click', closeWizModal);
wizCancelBtn.addEventListener('click', closeWizModal);

wizModalOverlay.addEventListener('click', (e) => {
  if (e.target === wizModalOverlay) closeWizModal();
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && !wizModalOverlay.hidden) closeWizModal();
});

wizForm.addEventListener('submit', async (e) => {
  e.preventDefault();

  const payload = {
    name: wizNameInput.value.trim(),
    type: wizTypeField.value,
    baseUrl: wizBaseUrlInput.value.trim(),
    clientId: wizClientIdInput.value.trim(),
    clientSecret: wizClientSecretInput.value,
    mcpServer: wizMcpServerInput.value.trim(),
  };

  const url = editingId ? `${API_BASE}/${editingId}` : API_BASE;
  const method = editingId ? 'PUT' : 'POST';

  try {
    const res = await fetch(url, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!res.ok) {
      const body = await res.json().catch(() => ({}));
      throw new Error(body.error || 'Request failed');
    }
    const saved = await res.json();
    if (editingId) {
      wizIntegrations = wizIntegrations.map((wi) => (wi.id === saved.id ? saved : wi));
    } else {
      wizIntegrations.push(saved);
    }
    renderWizCard();
    closeWizModal();
  } catch (err) {
    alert(err.message || 'Something went wrong saving the Wiz integration.');
  }
});

async function handleDeleteWiz(wi) {
  if (!confirm(`Delete "${wi.name}"? This will remove the stored credentials.`)) return;

  try {
    const res = await fetch(`${API_BASE}/${wi.id}`, { method: 'DELETE' });
    if (!res.ok && res.status !== 204) throw new Error('Failed to delete integration');
    wizIntegrations = wizIntegrations.filter((item) => item.id !== wi.id);
    renderWizCard();
  } catch (err) {
    alert(err.message || 'Something went wrong deleting the Wiz integration.');
  }
}

loadWizIntegrations();

// AI Agents (imported dataset, browsed via a searchable/paginated table)
const AGENTS_API = '/api/agents';
const AGENTS_PAGE_SIZE = 50;

const agentsSummary = document.getElementById('agentsSummary');
const agentsSearchInput = document.getElementById('agentsSearch');
const agentsTableBody = document.getElementById('agentsTableBody');
const agentsPrevBtn = document.getElementById('agentsPrevBtn');
const agentsNextBtn = document.getElementById('agentsNextBtn');
const agentsPageInfo = document.getElementById('agentsPageInfo');

let agentsOffset = 0;
let agentsSearchTerm = '';
let agentsSearchDebounce = null;

async function loadAgents() {
  agentsSummary.textContent = 'Loading agents...';

  const params = new URLSearchParams({
    limit: String(AGENTS_PAGE_SIZE),
    offset: String(agentsOffset),
  });
  if (agentsSearchTerm) params.set('search', agentsSearchTerm);

  try {
    const res = await fetch(`${AGENTS_API}?${params.toString()}`);
    if (!res.ok) throw new Error('Failed to load agents');
    const data = await res.json();
    renderAgentsTable(data);
  } catch (err) {
    console.error(err);
    agentsSummary.textContent = 'Unable to load agents.';
    agentsTableBody.innerHTML = '';
  }
}

function renderAgentsTable(data) {
  const { items, total, limit, offset } = data;

  agentsTableBody.innerHTML = '';
  items.forEach((agent) => {
    const row = document.createElement('tr');
    row.innerHTML = `
      <td>${escapeHtml(agent.name)}</td>
      <td class="cell-muted">${escapeHtml(agent.id)}</td>
      <td class="cell-muted">${escapeHtml(agent.agenticOverlayId)}</td>
      <td class="cell-muted cell-truncate" title="${escapeHtml(agent.externalId)}">${escapeHtml(agent.externalId)}</td>
      <td>${escapeHtml(agent.nativeType || agent.type)}</td>
      <td>${escapeHtml(agent.technologyName)}</td>
      <td>${escapeHtml(agent.cloudPlatform)}</td>
      <td>${escapeHtml(agent.status)}</td>
      <td>${escapeHtml(agent.region)}</td>
      <td>${escapeHtml(agent.projects)}</td>
      <td class="cell-muted">${escapeHtml(formatDate(agent.firstSeen))}</td>
      <td>${escapeHtml(agent.source)}</td>
      <td>
        <button type="button" class="risk-badge${agent.risks > 0 ? ' has-risks' : ''}" data-action="risks">${agent.risks}</button>
      </td>
      <td><span class="${riskScoreBadgeClass(agent.riskScore)}">${agent.riskScore}</span></td>
      <td>
        <label class="monitor-toggle">
          <input type="checkbox" data-action="monitor" ${agent.monitor ? 'checked' : ''}>
          <span class="track"></span>
          <span class="thumb"></span>
        </label>
      </td>
      <td><span class="${killSwitchBadgeClass(agent.killSwitchAction)}">${escapeHtml(agent.killSwitchAction)}</span></td>
    `;
    row.querySelector('[data-action="risks"]').addEventListener('click', () => openRiskModal(agent));
    row.querySelector('[data-action="monitor"]').addEventListener('change', (e) => handleMonitorToggle(agent, e.target));
    agentsTableBody.appendChild(row);
  });

  if (total === 0) {
    agentsSummary.textContent = agentsSearchTerm
      ? `No agents match "${agentsSearchTerm}".`
      : 'No agents found.';
  } else {
    agentsSummary.textContent = `${total.toLocaleString()} AI agent${total === 1 ? '' : 's'} found.`;
  }

  const start = total === 0 ? 0 : offset + 1;
  const end = Math.min(offset + limit, total);
  agentsPageInfo.textContent = total === 0 ? '' : `${start}-${end} of ${total.toLocaleString()}`;
  agentsPrevBtn.disabled = offset <= 0;
  agentsNextBtn.disabled = end >= total;
}

function formatDate(value) {
  if (!value) return '';
  const d = new Date(value);
  if (isNaN(d.getTime())) return value;
  return d.toLocaleDateString();
}

function killSwitchBadgeClass(action) {
  if (action === 'deactivated') return 'kill-switch-badge is-deactivated';
  if (action === 'reactivated') return 'kill-switch-badge is-reactivated';
  return 'kill-switch-badge';
}

function riskScoreBadgeClass(score) {
  if (score >= 70) return 'severity-badge severity-high';
  if (score >= 50) return 'severity-badge severity-medium';
  if (score > 0) return 'severity-badge severity-low';
  return 'severity-badge severity-informational';
}

agentsSearchInput.addEventListener('input', () => {
  clearTimeout(agentsSearchDebounce);
  agentsSearchDebounce = setTimeout(() => {
    agentsSearchTerm = agentsSearchInput.value.trim();
    agentsOffset = 0;
    loadAgents();
  }, 300);
});

agentsPrevBtn.addEventListener('click', () => {
  agentsOffset = Math.max(0, agentsOffset - AGENTS_PAGE_SIZE);
  loadAgents();
});

agentsNextBtn.addEventListener('click', () => {
  agentsOffset += AGENTS_PAGE_SIZE;
  loadAgents();
});

// Risk detail modal
const riskModalOverlay = document.getElementById('riskModalOverlay');
const riskModalTitle = document.getElementById('riskModalTitle');
const riskModalBody = document.getElementById('riskModalBody');
const riskModalClose = document.getElementById('riskModalClose');

function openRiskModal(agent) {
  riskModalTitle.textContent = `Risks — ${agent.name}`;

  if (agent.risks > 0) {
    riskModalBody.innerHTML = `<p>${agent.risks} risk${agent.risks === 1 ? '' : 's'} found for <span class="risk-agent-name">${escapeHtml(agent.name)}</span>.</p>`;
  } else {
    riskModalBody.innerHTML = `
      <div class="risk-empty-icon">
        <svg width="20" height="20" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
          <path d="M4 10.5L8 14.5L16 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
      </div>
      <p>No risks found for <span class="risk-agent-name">${escapeHtml(agent.name)}</span>.</p>
    `;
  }
  riskModalOverlay.hidden = false;
}

function closeRiskModal() {
  riskModalOverlay.hidden = true;
}

riskModalClose.addEventListener('click', closeRiskModal);
riskModalOverlay.addEventListener('click', (e) => {
  if (e.target === riskModalOverlay) closeRiskModal();
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && !riskModalOverlay.hidden) closeRiskModal();
});

// Monitor toggle
async function handleMonitorToggle(agent, checkbox) {
  const nextValue = checkbox.checked;
  checkbox.disabled = true;
  try {
    const res = await fetch(`${AGENTS_API}/${encodeURIComponent(agent.id)}/monitor`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ monitor: nextValue }),
    });
    if (!res.ok) throw new Error('Failed to update monitor status');
    agent.monitor = nextValue;
  } catch (err) {
    checkbox.checked = !nextValue;
    alert(err.message || 'Something went wrong updating monitor status.');
  } finally {
    checkbox.disabled = false;
  }
}

// Policies (imported dataset, browsed via a searchable/paginated table)
const POLICIES_API = '/api/policies';
const POLICIES_PAGE_SIZE = 50;

const policiesSummary = document.getElementById('policiesSummary');
const policiesSearchInput = document.getElementById('policiesSearch');
const policiesTableBody = document.getElementById('policiesTableBody');
const policiesPrevBtn = document.getElementById('policiesPrevBtn');
const policiesNextBtn = document.getElementById('policiesNextBtn');
const policiesPageInfo = document.getElementById('policiesPageInfo');

let policiesOffset = 0;
let policiesSearchTerm = '';
let policiesSearchDebounce = null;

async function loadPolicies() {
  policiesSummary.textContent = 'Loading policies...';

  const params = new URLSearchParams({
    limit: String(POLICIES_PAGE_SIZE),
    offset: String(policiesOffset),
  });
  if (policiesSearchTerm) params.set('search', policiesSearchTerm);

  try {
    const res = await fetch(`${POLICIES_API}?${params.toString()}`);
    if (!res.ok) throw new Error('Failed to load policies');
    const data = await res.json();
    renderPoliciesTable(data);
  } catch (err) {
    console.error(err);
    policiesSummary.textContent = 'Unable to load policies.';
    policiesTableBody.innerHTML = '';
  }
}

function severityBadgeClass(severity) {
  const key = (severity || '').toLowerCase();
  return ['critical', 'high', 'medium', 'low', 'informational'].includes(key)
    ? `severity-badge severity-${key}`
    : 'severity-badge';
}

function renderPoliciesTable(data) {
  const { items, total, limit, offset } = data;

  policiesTableBody.innerHTML = '';
  items.forEach((policy) => {
    const row = document.createElement('tr');
    row.innerHTML = `
      <td>${escapeHtml(policy.name)}</td>
      <td class="cell-muted">${escapeHtml(policy.policyId)}</td>
      <td>${escapeHtml(policy.policyType)}</td>
      <td><span class="update-type-badge">${escapeHtml(policy.updateType)}</span></td>
      <td><span class="${severityBadgeClass(policy.severity)}">${escapeHtml(policy.severity)}</span></td>
      <td>${escapeHtml(policy.cloudPlatform)}</td>
      <td class="cell-muted">${escapeHtml(formatDate(policy.releasedAt))}</td>
      <td class="cell-muted">${escapeHtml(formatDate(policy.applyDate))}</td>
      <td>
        <label class="monitor-toggle">
          <input type="checkbox" data-action="enabled" ${policy.enabled ? 'checked' : ''}>
          <span class="track"></span>
          <span class="thumb"></span>
        </label>
      </td>
      <td>
        <button type="button" class="integration-action-btn" data-action="details">View</button>
      </td>
    `;
    row.querySelector('[data-action="details"]').addEventListener('click', () => openRegoModal(policy));
    row.querySelector('[data-action="enabled"]').addEventListener('change', (e) => handlePolicyEnabledToggle(policy, e.target));
    policiesTableBody.appendChild(row);
  });

  if (total === 0) {
    policiesSummary.textContent = policiesSearchTerm
      ? `No policies match "${policiesSearchTerm}".`
      : 'No policies found.';
  } else {
    policiesSummary.textContent = `${total.toLocaleString()} polic${total === 1 ? 'y' : 'ies'} found.`;
  }

  const start = total === 0 ? 0 : offset + 1;
  const end = Math.min(offset + limit, total);
  policiesPageInfo.textContent = total === 0 ? '' : `${start}-${end} of ${total.toLocaleString()}`;
  policiesPrevBtn.disabled = offset <= 0;
  policiesNextBtn.disabled = end >= total;
}

policiesSearchInput.addEventListener('input', () => {
  clearTimeout(policiesSearchDebounce);
  policiesSearchDebounce = setTimeout(() => {
    policiesSearchTerm = policiesSearchInput.value.trim();
    policiesOffset = 0;
    loadPolicies();
  }, 300);
});

policiesPrevBtn.addEventListener('click', () => {
  policiesOffset = Math.max(0, policiesOffset - POLICIES_PAGE_SIZE);
  loadPolicies();
});

policiesNextBtn.addEventListener('click', () => {
  policiesOffset += POLICIES_PAGE_SIZE;
  loadPolicies();
});

// Enabled toggle
async function handlePolicyEnabledToggle(policy, checkbox) {
  const nextValue = checkbox.checked;
  checkbox.disabled = true;
  try {
    const res = await fetch(`${POLICIES_API}/${encodeURIComponent(policy.id)}/enabled`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled: nextValue }),
    });
    if (!res.ok) throw new Error('Failed to update policy status');
    policy.enabled = nextValue;
  } catch (err) {
    checkbox.checked = !nextValue;
    alert(err.message || 'Something went wrong updating the policy status.');
  } finally {
    checkbox.disabled = false;
  }
}

// Rego policy detail modal
const regoModalOverlay = document.getElementById('regoModalOverlay');
const regoModalTitle = document.getElementById('regoModalTitle');
const regoModalSubtitle = document.getElementById('regoModalSubtitle');
const regoModalClose = document.getElementById('regoModalClose');
const regoCode = document.getElementById('regoCode');
const regoCopyBtn = document.getElementById('regoCopyBtn');

function openRegoModal(policy) {
  regoModalTitle.textContent = policy.name;
  regoModalSubtitle.textContent = policy.policyId;
  regoCode.textContent = policy.regoPolicy;
  regoCopyBtn.textContent = 'Copy';
  regoModalOverlay.hidden = false;
}

function closeRegoModal() {
  regoModalOverlay.hidden = true;
}

regoModalClose.addEventListener('click', closeRegoModal);
regoModalOverlay.addEventListener('click', (e) => {
  if (e.target === regoModalOverlay) closeRegoModal();
});
document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && !regoModalOverlay.hidden) closeRegoModal();
});

regoCopyBtn.addEventListener('click', async () => {
  try {
    await navigator.clipboard.writeText(regoCode.textContent);
    regoCopyBtn.textContent = 'Copied!';
  } catch (err) {
    console.error(err);
    regoCopyBtn.textContent = 'Copy failed';
  } finally {
    setTimeout(() => {
      regoCopyBtn.textContent = 'Copy';
    }, 1500);
  }
});

// Dashboard: Milestone 1 Reporting (Mapping, Measuring, Monitoring, Kill Switch)
const dashboardStatEls = {
  agentsMapped: document.getElementById('statAgentsMapped'),
  policiesMapped: document.getElementById('statPoliciesMapped'),
  agentsLow24h: document.getElementById('statAgentsLow24h'),
  agentsMedium24h: document.getElementById('statAgentsMedium24h'),
  agentsHigh24h: document.getElementById('statAgentsHigh24h'),
  agentsMonitored24h: document.getElementById('statAgentsMonitored24h'),
  agentsDisabled24h: document.getElementById('statAgentsDisabled24h'),
};

const riskTrendChartEl = document.getElementById('riskTrendChart');
const riskTrendLegendEl = document.getElementById('riskTrendLegend');
const monitoredTrendChartEl = document.getElementById('monitoredTrendChart');
const disabledTrendChartEl = document.getElementById('disabledTrendChart');
const highRiskTableBody = document.getElementById('highRiskTableBody');

async function loadDashboardReporting() {
  try {
    const res = await fetch('/api/dashboard/reporting');
    if (!res.ok) throw new Error('Failed to load dashboard reporting');
    const data = await res.json();
    renderDashboardReporting(data);
  } catch (err) {
    console.error(err);
    Object.values(dashboardStatEls).forEach((el) => {
      el.textContent = '—';
    });
  }
}

function renderDashboardReporting(data) {
  dashboardStatEls.agentsMapped.textContent = data.mapping.agentsMapped.toLocaleString();
  dashboardStatEls.policiesMapped.textContent = data.mapping.policiesMapped.toLocaleString();
  dashboardStatEls.agentsLow24h.textContent = data.measuring.agentsLow24h.toLocaleString();
  dashboardStatEls.agentsMedium24h.textContent = data.measuring.agentsMedium24h.toLocaleString();
  dashboardStatEls.agentsHigh24h.textContent = data.measuring.agentsHigh24h.toLocaleString();
  dashboardStatEls.agentsMonitored24h.textContent = data.monitoring.agentsMonitored24h.toLocaleString();
  dashboardStatEls.agentsDisabled24h.textContent = data.killSwitch.agentsDisabled24h.toLocaleString();

  const riskSeries = [
    { key: 'low', label: 'Low', color: '#2a6df4' },
    { key: 'medium', label: 'Medium', color: '#d97706' },
    { key: 'high', label: 'High', color: '#dc2626' },
  ];
  riskTrendLegendEl.innerHTML = riskSeries
    .map(
      (s) => `
      <span class="chart-legend-item">
        <span class="chart-legend-swatch" style="background:${s.color}"></span>${s.label}
      </span>
    `
    )
    .join('');
  riskTrendChartEl.innerHTML = buildLineChartSVG(data.measuring.riskTrend, riskSeries);

  monitoredTrendChartEl.innerHTML = buildBarChartSVG(data.monitoring.monitoredTrend, '#4f46e5');
  disabledTrendChartEl.innerHTML = buildBarChartSVG(data.killSwitch.disabledTrend, '#dc2626');

  highRiskTableBody.innerHTML = '';
  data.measuring.highRiskAgents.forEach((agent) => {
    const row = document.createElement('tr');
    row.innerHTML = `
      <td>${escapeHtml(agent.name)}</td>
      <td><span class="${riskScoreBadgeClass(agent.riskScore)}">${agent.riskScore}</span></td>
      <td>${escapeHtml(agent.source)}</td>
      <td>${agent.monitor ? 'On' : 'Off'}</td>
      <td><span class="${killSwitchBadgeClass(agent.killSwitchAction)}">${escapeHtml(agent.killSwitchAction)}</span></td>
    `;
    highRiskTableBody.appendChild(row);
  });
}

function formatShortDate(isoDate) {
  const d = new Date(isoDate + 'T00:00:00');
  return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}

// Picks evenly-spaced x-axis label indices, always including the last point,
// but skipping the last regular tick if it would crowd the final label.
function shouldShowLabel(i, total, labelEvery) {
  if (i === total - 1) return true;
  if (i % labelEvery !== 0) return false;
  return total - 1 - i >= labelEvery / 2;
}

// Renders a simple multi-series line chart (no external dependencies).
function buildLineChartSVG(points, series) {
  if (!points || points.length === 0) {
    return '<div class="chart-empty">No data yet.</div>';
  }

  const width = 760;
  const height = 220;
  const padLeft = 32;
  const padRight = 12;
  const padTop = 12;
  const padBottom = 28;
  const plotWidth = width - padLeft - padRight;
  const plotHeight = height - padTop - padBottom;

  const maxVal = Math.max(1, ...points.flatMap((p) => series.map((s) => p[s.key])));
  const xStep = points.length > 1 ? plotWidth / (points.length - 1) : 0;
  const xAt = (i) => padLeft + i * xStep;
  const yAt = (v) => padTop + plotHeight - (v / maxVal) * plotHeight;

  const gridLines = [0, 0.5, 1]
    .map((frac) => {
      const y = padTop + plotHeight * (1 - frac);
      const label = Math.round(maxVal * frac);
      return `
        <line x1="${padLeft}" y1="${y}" x2="${width - padRight}" y2="${y}" stroke="#eceef1" stroke-width="1"/>
        <text x="${padLeft - 8}" y="${y + 4}" text-anchor="end" font-size="10" fill="#8b949e">${label}</text>
      `;
    })
    .join('');

  const labelEvery = Math.ceil(points.length / 7);
  const xLabels = points
    .map((p, i) => {
      if (!shouldShowLabel(i, points.length, labelEvery)) return '';
      return `<text x="${xAt(i)}" y="${height - 8}" text-anchor="middle" font-size="10" fill="#8b949e">${formatShortDate(p.date)}</text>`;
    })
    .join('');

  const seriesPaths = series
    .map((s) => {
      const linePoints = points.map((p, i) => `${xAt(i)},${yAt(p[s.key])}`).join(' ');
      const dots = points
        .map((p, i) => `<circle cx="${xAt(i)}" cy="${yAt(p[s.key])}" r="2.5" fill="${s.color}"/>`)
        .join('');
      return `<polyline points="${linePoints}" fill="none" stroke="${s.color}" stroke-width="2" stroke-linejoin="round" stroke-linecap="round"/>${dots}`;
    })
    .join('');

  return `
    <svg viewBox="0 0 ${width} ${height}" width="100%" height="${height}" role="img">
      ${gridLines}
      ${seriesPaths}
      ${xLabels}
    </svg>
  `;
}

// Renders a simple single-series bar chart (no external dependencies).
function buildBarChartSVG(points, color) {
  if (!points || points.length === 0) {
    return '<div class="chart-empty">No data yet.</div>';
  }

  const width = 760;
  const height = 180;
  const padLeft = 32;
  const padRight = 12;
  const padTop = 12;
  const padBottom = 28;
  const plotWidth = width - padLeft - padRight;
  const plotHeight = height - padTop - padBottom;

  const maxVal = Math.max(1, ...points.map((p) => p.count));
  const slot = plotWidth / points.length;
  const barWidth = Math.max(2, slot * 0.6);

  const gridLine = `
    <line x1="${padLeft}" y1="${padTop}" x2="${width - padRight}" y2="${padTop}" stroke="#eceef1" stroke-width="1"/>
    <line x1="${padLeft}" y1="${padTop + plotHeight}" x2="${width - padRight}" y2="${padTop + plotHeight}" stroke="#eceef1" stroke-width="1"/>
    <text x="${padLeft - 8}" y="${padTop + 4}" text-anchor="end" font-size="10" fill="#8b949e">${maxVal}</text>
    <text x="${padLeft - 8}" y="${padTop + plotHeight + 4}" text-anchor="end" font-size="10" fill="#8b949e">0</text>
  `;

  const labelEvery = Math.ceil(points.length / 8);
  const bars = points
    .map((p, i) => {
      const barHeight = (p.count / maxVal) * plotHeight;
      const x = padLeft + i * slot + (slot - barWidth) / 2;
      const y = padTop + plotHeight - barHeight;
      const label = shouldShowLabel(i, points.length, labelEvery)
        ? `<text x="${x + barWidth / 2}" y="${height - 8}" text-anchor="middle" font-size="10" fill="#8b949e">${formatShortDate(p.date)}</text>`
        : '';
      return `<rect x="${x}" y="${y}" width="${barWidth}" height="${Math.max(0, barHeight)}" rx="1.5" fill="${color}"/>${label}`;
    })
    .join('');

  return `
    <svg viewBox="0 0 ${width} ${height}" width="100%" height="${height}" role="img">
      ${gridLine}
      ${bars}
    </svg>
  `;
}

loadDashboardReporting();
