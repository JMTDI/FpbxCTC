const domainInput = document.getElementById('domain');
const apiKeyInput = document.getElementById('apiKey');
const agentInput  = document.getElementById('agentNumber');
const saveBtn     = document.getElementById('saveBtn');
const syncBtn     = document.getElementById('syncBtn');
const statusEl    = document.getElementById('status');

// ── Load saved settings on open ───────────────────────────────────────────
chrome.storage.sync.get(['domain', 'apiKey', 'agentNumber'], (items) => {
  if (items.domain)      domainInput.value = items.domain;
  if (items.apiKey)      apiKeyInput.value = items.apiKey;
  if (items.agentNumber) agentInput.value  = items.agentNumber;
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

// ── Sync from Desktop App (Native Messaging) ──────────────────────────────
syncBtn.addEventListener('click', () => {
  setStatus('Syncing from desktop app…', '');
  chrome.runtime.sendNativeMessage(
    'com.fpbxctc.host',
    { action: 'getConfig' },
    (resp) => {
      if (chrome.runtime.lastError) {
        setStatus('Sync failed: ' + chrome.runtime.lastError.message, 'err');
        return;
      }
      if (resp && resp.error) {
        setStatus('Sync failed: ' + resp.error, 'err');
        return;
      }
      if (resp.domain)      domainInput.value = resp.domain;
      if (resp.apiKey)      apiKeyInput.value = resp.apiKey;
      if (resp.agentNumber) agentInput.value  = resp.agentNumber;
      // Auto-save the synced values
      chrome.storage.sync.set({
        domain:      resp.domain      || '',
        apiKey:      resp.apiKey      || '',
        agentNumber: resp.agentNumber || '',
      }, () => {
        setStatus('Synced from desktop app ✓', 'ok');
        setTimeout(() => setStatus(''), 3000);
      });
    },
  );
});
