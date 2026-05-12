const domainInput = document.getElementById('domain');
const apiKeyInput = document.getElementById('apiKey');
const agentInput  = document.getElementById('agentNumber');
const saveBtn     = document.getElementById('saveBtn');
const statusEl    = document.getElementById('status');

// ── Load saved settings, falling back to bootstrap.json if first install ──
chrome.storage.sync.get(['domain', 'apiKey', 'agentNumber'], (items) => {
  if (items.domain || items.apiKey || items.agentNumber) {
    // Already configured — use stored values
    if (items.domain)      domainInput.value = items.domain;
    if (items.apiKey)      apiKeyInput.value = items.apiKey;
    if (items.agentNumber) agentInput.value  = items.agentNumber;
  } else {
    // First install — try bootstrap.json written by FpbxCTC.exe
    fetch(chrome.runtime.getURL('bootstrap.json'))
      .then((r) => r.ok ? r.json() : null)
      .then((data) => {
        if (!data) return;
        const domain      = (data.domain       || '').trim();
        const apiKey      = (data.api_key       || '').trim();
        const agentNumber = (data.agent_number  || '').replace(/\D/g, '');
        if (domain || apiKey || agentNumber) {
          domainInput.value = domain;
          apiKeyInput.value = apiKey;
          agentInput.value  = agentNumber;
          // Auto-save so subsequent opens use storage
          chrome.storage.sync.set({ domain, apiKey, agentNumber });
          setStatus('Settings loaded from desktop app ✓', 'ok');
          setTimeout(() => setStatus(''), 4000);
        }
      })
      .catch(() => {}); // bootstrap.json absent — fresh install, ignore
  }
});

// ── Save on button click ──────────────────────────────────────────────────
saveBtn.addEventListener('click', () => {
  const domain      = domainInput.value.trim().replace(/^https?:\/\//i, '').replace(/\/$/, '');
  const apiKey      = apiKeyInput.value.trim();
  const agentNumber = agentInput.value.trim().replace(/\D/g, '');

  if (!domain || !apiKey || !agentNumber) {
    setStatus('All three fields are required.', 'err');
    return;
  }

  chrome.storage.sync.set({ domain, apiKey, agentNumber }, () => {
    // Update the displayed domain in case we stripped the scheme
    domainInput.value = domain;
    agentInput.value  = agentNumber;
    setStatus('Saved ✓', 'ok');
    setTimeout(() => setStatus(''), 3000);
  });
});

// ── Allow Enter key in any field to save ─────────────────────────────────
[domainInput, apiKeyInput, agentInput].forEach((el) => {
  el.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') saveBtn.click();
  });
});

function setStatus(msg, cls) {
  statusEl.textContent = msg;
  statusEl.className = 'status ' + (cls || '');
}
